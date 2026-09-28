# Internals
Provides some basic knowledge needed to be productive with embedding this language.

## Scoping

Understanding this is crucial to knowing how variables are resolved.
```go
{
    // universal-statics.
    // visible to all packages implicitly (no `imports` needed).
    // cannot be reassigned by the user.

    {
        // package-statics.
        // visible only to the current package.
        // e.g. `package xyz imports("time")`.
        // will make `time` available in the current `xyz` package.
        // cannot be reassigned by the user.

        {
            // package-globals.
            // visible in the current package and, through its name, to importers.
            // e.g. `isWorldFlat := false`.
            // can be reassigned if prefixed with `var`.
            // initialized in source order when the package loads; functions and structs are hoisted.

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
The `evie` package wraps all of this in an `Engine`; this is the same thing on the `vm` level.
```go
func main() {
	// the prelude: visible to every package without an import
	prelude := vm.NewPackage("prelude")
	prelude.Set("pi", vm.BoxNumber(3.14159))

	// packages that scripts import by name, e.g. `package foo imports("io")`
	resolver := func(name string) (*vm.Package, error) {
		switch name {
		case "io":
			return io.Construct(), nil
		case "time":
			return time.Construct(), nil
		}
		return nil, fmt.Errorf("package '%v' not found", name)
	}

	evm := vm.New(vm.Options{Prelude: prelude, Resolver: resolver})

	_, err := evm.EvalString(`package main imports("io", "time")

	fn add(a, b) {
		io.println("add called in the evie world")
		time.wait(pi * 1000) // wait 3.14 seconds
		return a + b
	}`)
	if err != nil {
		panic(err)
	}

	// the convenient call: Go arguments are boxed, a fiber is borrowed
	result, err := evm.Call("main.add", 3, 2)

	// the explicit call: a looked-up value and evie values
	add, _ := evm.Lookup("main.add")
	result, exc := add.Call(nil, vm.BoxNumber(3), vm.BoxNumber(2))
}
```
