# Internals
Provides some basic knowledge needed to be productive with embedding this language.

## Scoping

Understanding this is crucial to knowing how variables are resolved.
```go
{
    // the prelude: the builtin types such as `string`, `number` and `error`, plus what the host adds.
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

## Compiler fast paths

The compiler turns each AST node into a Go closure. A few rules keep the hot paths fast; check `go build -gcflags=-m=1 ./vm` and the benchmark suite when touching them.

- **Operands that need no evaluation.** `evaluate` returns an `operand`: a `Value` for a constant, a `local` for a stack slot, a `Global` for a package binding, or nil. Emitters specialise on these, so `x + 1` reads the slot and adds the constant without calling another closure.
- **Generated operator matrix.** Go only inlines `Value.Add` and its siblings when they are called directly from the closure; dispatching through a table of functions costs 10% to 40% on numeric loops. So `ops_gen.go` spells out every operator for every operand shape, including compound assignments and comparisons used as conditions, which compare numbers without boxing a bool. It is written by `gen_ops.go`; edit the generator and run `go generate ./vm`, never the output.
- **Frames spelled out.** Go will not inline a frame-setup helper, and calling one costs 5% to 17% on call-heavy code, so the call protocol is written out in each entry point (`emitCall`, `UserFn.invoke`, `invokeMethod`). Arity checks are inlinable through precomputed minimum and maximum counts.
- **Per-site method cache.** A member call remembers the struct type it last saw and the method it resolved to, so a monomorphic call site skips the lookup.
- **Code layout moves numbers.** The same code emitted in a different order has shifted float loops by 7% either way. Run an A/B at least twice and judge by the suite's geometric mean, not one benchmark.
