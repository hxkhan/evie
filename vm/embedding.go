package vm

import (
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
// Otherwise, it checks package "main", universal statics, and builtins in order.
func (vm *Instance) Lookup(path string) (Value, bool) {
	if idx := strings.IndexByte(path, '.'); idx >= 0 {
		return vm.Resolve(path[:idx], path[idx+1:])
	}
	if pkgMain := vm.GetPackage("main"); pkgMain != nil {
		if v, ok := pkgMain.Get(path); ok {
			return v, true
		}
	}
	if vm.cp.statics != nil {
		if val, ok := vm.cp.statics[path]; ok && val != nil {
			return *val, true
		}
	}
	if b, ok := builtins[path]; ok && b != nil {
		return *b, true
	}
	return Value{}, false
}

// SetStatic registers a universal static Value available across all packages.
func (vm *Instance) SetStatic(name string, val Value) {
	if vm.cp.statics == nil {
		vm.cp.statics = make(map[string]*Value)
	}
	vm.cp.statics[name] = val.Allocate()
}

// SetGlobal sets a symbol in a loaded package. Returns false if the package is not loaded.
func (vm *Instance) SetGlobal(pkgName, symName string, val Value) bool {
	pkg := vm.GetPackage(pkgName)
	if pkg == nil {
		return false
	}
	pkg.SetSymbol(symName, val)
	return true
}
