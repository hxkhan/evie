# Internals
Provides some basic knowledge needed to be productive with embedding this language.

## Scoping

Understanding this is crucial to knowing how variables are resolved.
```go
{
    // universal-statics.
    // visible to all packages implicitly (no `imports` needed).
    // cannot be reassigned.

    {
        // package-statics.
        // visible only to the current package.
        // e.g. `package xyz imports("time")`.
        // will make `time` available in the current `xyz` package.
        // cannot be reassigned.

        {
            // package-globals.
            // visible only to the current package.
            // e.g. `isWorldFlat := false`.
            // will make `isWorldFlat` visible in the current package.
            // can be reassigned.

            {
                // package-closures.
                // visible to the current closure and other nested closures.
                // can be reassigned.
            }
        }
    }
}
```
At each layer, you *can* shadow symbols in the previous scope.

### Example
Here is an example of all of these
```go
func main() {
	// universal-statics
	statics := map[string]*vm.Value{
		"pi":   vm.BoxFloat64(3.14159).Allocate(), // constant value
		"time": time.Construct().Box().Allocate(), // already instantiated package
	}

	// package-statics for packages that import them via the header
	// e.g. package foo imports("bar")
	resolver := func(name string) vm.Package {
		switch name {
		case "io":
			return io.Construct()
		}
		panic(fmt.Errorf("constructor not found for '%v'", name))
	}

	// create a vm with our options
	evm := vm.New(vm.Options{
		UniversalStatics: statics,
		ImportResolver:   resolver,
	})

	// evaluate our script
	result, err := evm.EvalScript([]byte(
		`package main imports("io")
		
		fn add(a, b) {
            io.println("add called in the evie world")

			io.println("about to start awaiting PI seconds")
            await time.timer(pi * 1000) // wait 3.14 seconds
            return a + b
		}`,
	))

	// check errors
	if err != nil {
		panic(err)
	}

	// print the result (nothing in this case)
	if !result.IsNil() {
		fmt.Println(result)
	}

	// get a reference to the main package
	pkgMain := evm.GetPackage("main")
	if pkgMain == nil {
		panic("package main not found")
	}

	// get a reference to the main symbol
	symAdd, exists := pkgMain.GetSymbol("add")
	if !exists {
		panic("symbol fib not found")
	}

	// type assert it
	add, ok := symAdd.AsUserFn()
	if !ok {
		panic("symbol fib is not a function")
	}

	// call it & check for errors
	result, err = add.Call(vm.BoxFloat64(3), vm.BoxFloat64(2))
	if err != nil {
		panic(err)
	}

	// print the result
	if !result.IsNil() {
		fmt.Println(result)
	}
}
```