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
	statics := map[string]vm.Value{
		"pi":     vm.BoxFloat64(3.14159),
		"time":   vm.ConstructPackage(time.Constructor).Box(),
		"before": vm.BoxString("Hello World"),
	}

	// package-statics for packages that import them via the header
	// e.g. package foo imports("bar")
	constructors := []vm.PackageContructor{
		fs.Constructor,
		json.Constructor,
	}

	// create a vm with our options
	evm := vm.New(vm.Options{
		UniversalStatics:   statics,
		PackageContructors: constructors,
	})

	// evaluate our script
	result, err := evm.EvalScript([]byte(
		`package main imports("fs")
		
		fn main() {
            after := "Bye World"

            echo before
            await time.timer(pi * 1000)
            echo after

            return await fs.readFile("file.txt")
		}`,
	))

	// check errors
	if err != nil {
		panic(err)
	}

	// print the result (nothing in this case)
	if result.IsTruthy() {
		fmt.Println(result)
	}

	// get a reference to the main package
	pkgMain := evm.GetPackage("main")
	if pkgMain == nil {
		panic("package main not found")
	}

	// get a reference to the main symbol
	symMain, exists := pkgMain.GetSymbol("main")
	if !exists {
		panic("symbol fib not found")
	}

	// type assert it
	main, ok := symMain.AsUserFn()
	if !ok {
		panic("symbol fib is not a function")
	}

	// call it & check for errors
	result, err = main.Call()
	if err != nil {
		panic(err)
	}

	// print the result
	if !result.IsNull() {
		if buffer, ok := result.AsBuffer(); ok {
			fmt.Println(string(buffer))
		}
	}
}
```