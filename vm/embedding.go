package vm

import (
	"fmt"
	"strings"
)

// EvalString parses and executes an Evie script string, returning the result Value.
func (vm *Instance) EvalString(script string) (Value, error) {
	return vm.EvalScript([]byte(script))
}

// Resolve looks up a symbol's Value directly from a loaded package by package name and symbol name.
func (vm *Instance) Resolve(pkgName, symName string) (Value, bool) {
	pkg := vm.GetPackage(pkgName)
	if pkg == nil {
		return Value{}, false
	}
	return pkg.Get(symName)
}

// Lookup resolves a symbol Value from the VM.
// If path contains a dot (e.g. "main.add", "io.println"), it resolves the symbol from that package.
// Otherwise, it checks package "main" and then the prelude.
func (vm *Instance) Lookup(path string) (Value, bool) {
	if idx := strings.IndexByte(path, '.'); idx >= 0 {
		return vm.Resolve(path[:idx], path[idx+1:])
	}
	if pkgMain := vm.GetPackage("main"); pkgMain != nil {
		if v, ok := pkgMain.Get(path); ok {
			return v, true
		}
	}
	return vm.cp.prelude.Get(path)
}

// Prelude returns the package whose symbols every script sees without an import
func (vm *Instance) Prelude() *Package {
	return vm.cp.prelude
}

// SetStatic binds a symbol in the prelude, visible to every package
func (vm *Instance) SetStatic(name string, val Value) {
	vm.cp.prelude.Set(name, val)
}

// SetGlobal sets a symbol in a loaded package. Returns false if the package is not loaded.
func (vm *Instance) SetGlobal(pkgName, symName string, val Value) bool {
	pkg := vm.GetPackage(pkgName)
	if pkg == nil {
		return false
	}
	pkg.Set(symName, val)
	return true
}

// Call looks a callable up by path, such as "main.add", boxes the Go arguments
// with BoxAny and calls it on a borrowed fiber. It is the simplest way to call
// into a script from Go.
func (vm *Instance) Call(path string, args ...any) (Value, error) {
	fn, ok := vm.Lookup(path)
	if !ok {
		return Value{}, fmt.Errorf("symbol '%s' not found", path)
	}
	if !fn.IsCallable() {
		return Value{}, fmt.Errorf("symbol '%s' is a %s, not a callable", path, fn.TypeOf())
	}
	boxed := make([]Value, len(args))
	for i, arg := range args {
		v, err := BoxAny(arg)
		if err != nil {
			return Value{}, fmt.Errorf("argument %d: %w", i+1, err)
		}
		boxed[i] = v
	}
	return plain(fn.Call(nil, boxed...))
}
