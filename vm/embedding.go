package vm

import (
	"fmt"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/parser"
)

// Run parses and executes an Evie script string.
// If the script defines a package with a main() function, it automatically calls main().
// Otherwise, it returns the result of the evaluated script or block.
func (vm *Instance) Run(script string) (Value, error) {
	node, err := parser.Parse([]byte(script))
	if err != nil {
		return Value{}, err
	}

	result, err := vm.EvalNode(node)
	if err != nil {
		return result, err
	}

	// If this was a package definition, check if main() exists
	if _, isPackage := node.(ast.Package); isPackage {
		pkgMain := vm.GetPackage("main")
		if pkgMain != nil {
			if symMain, exists := pkgMain.GetSymbol("main"); exists {
				if fn, ok := symMain.AsUserFn(); ok {
					fbr := vm.NewFiber()
					res, exc := fn.Call(fbr)
					vm.WaitForNoActivity()
					if exc != nil {
						return res, exc
					}
					return res, nil
				}
			}
		}
	}

	return result, nil
}

// RunString is an alias for Run.
func (vm *Instance) RunString(script string) (Value, error) {
	return vm.Run(script)
}

// Set sets a global variable or function in the VM, accessible to all packages.
// val can be any standard Go type, slice, map, struct, or func(...).
func (vm *Instance) Set(name string, val any) error {
	boxed, err := BoxAny(val)
	if err != nil {
		return err
	}

	if vm.cp.statics == nil {
		vm.cp.statics = make(map[string]*Value)
	}
	vm.cp.statics[name] = boxed.Allocate()

	// Also update package main if already instantiated
	if pkgMain := vm.GetPackage("main"); pkgMain != nil {
		pkgMain.SetSymbol(name, boxed)
	}

	return nil
}

// Get retrieves a global symbol by name (from package main, universal statics, or builtins).
func (vm *Instance) Get(name string) (Value, bool) {
	if pkgMain := vm.GetPackage("main"); pkgMain != nil {
		if sym, exists := pkgMain.GetSymbol(name); exists {
			return *sym.Value, true
		}
	}
	if vm.cp.statics != nil {
		if val, exists := vm.cp.statics[name]; exists {
			return *val, true
		}
	}
	if b, exists := builtins[name]; exists {
		return *b, true
	}
	return Value{}, false
}

// Call invokes an Evie function by name (from package main or statics) with arguments automatically boxed from Go.
func (vm *Instance) Call(fnName string, args ...any) (Value, error) {
	return vm.CallIn("main", fnName, args...)
}

// CallIn invokes an Evie function located in a specific package pkgName.
func (vm *Instance) CallIn(pkgName, fnName string, args ...any) (Value, error) {
	var fnVal Value
	pkg := vm.GetPackage(pkgName)
	if pkg != nil {
		if sym, exists := pkg.GetSymbol(fnName); exists {
			fnVal = *sym.Value
		}
	}

	if fnVal.IsNil() && vm.cp.statics != nil {
		if val, exists := vm.cp.statics[fnName]; exists {
			fnVal = *val
		}
	}

	if fnVal.IsNil() {
		return Value{}, fmt.Errorf("function '%s' not found in package '%s'", fnName, pkgName)
	}

	boxedArgs := make([]Value, len(args))
	for i, arg := range args {
		b, err := BoxAny(arg)
		if err != nil {
			return Value{}, fmt.Errorf("failed to box argument %d: %w", i, err)
		}
		boxedArgs[i] = b
	}

	fbr := vm.NewFiber()
	res, exc := fnVal.Call(fbr, boxedArgs...)
	vm.WaitForNoActivity()
	if exc != nil {
		return res, exc
	}
	return res, nil
}
