package vm

import (
	"context"
	"fmt"
	"strings"
)

// resolveSymbol looks up a symbol's Value directly from a loaded package by package name and symbol name.
func (vm *Instance) resolveSymbol(pkgName, symName string) (Value, bool) {
	pkg := vm.Package(pkgName)
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
		return vm.resolveSymbol(path[:idx], path[idx+1:])
	}
	if pkgMain := vm.Package("main"); pkgMain != nil {
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

// Call looks a callable up by path, such as "main.add", boxes the Go arguments
// with BoxAny and calls it on a borrowed fiber. It is the simplest way to call
// into a script from Go.
func (vm *Instance) Call(path string, args ...any) (Value, error) {
	fn, boxed, err := vm.callable(path, args)
	if err != nil {
		return Value{}, err
	}
	return plain(fn.callGuarded(nil, boxed...))
}

// CallContext is Call bound to ctx: when ctx is done, the call stops with an
// error whose cause is ctx's cause, such as context.DeadlineExceeded. Tasks
// the call starts stop with it, also after it returns.
func (vm *Instance) CallContext(ctx context.Context, path string, args ...any) (Value, error) {
	fn, boxed, err := vm.callable(path, args)
	if err != nil {
		return Value{}, err
	}
	s := vm.rt.scope(ctx)
	defer s.drop()
	fbr := vm.borrowFiber(s)
	defer vm.releaseFiber(fbr)
	return plain(fn.callGuarded(fbr, boxed...))
}

// callable looks up a callable by path and boxes the arguments for it
func (vm *Instance) callable(path string, args []any) (Value, []Value, error) {
	fn, ok := vm.Lookup(path)
	if !ok {
		return Value{}, nil, fmt.Errorf("symbol '%s' not found", path)
	}
	if !fn.IsCallable() {
		return Value{}, nil, fmt.Errorf("symbol '%s' is a %s, not a callable", path, fn.TypeOf())
	}
	boxed := make([]Value, len(args))
	for i, arg := range args {
		v, err := BoxAny(arg)
		if err != nil {
			return Value{}, nil, fmt.Errorf("argument %d: %w", i+1, err)
		}
		boxed[i] = v
	}
	return fn, boxed, nil
}
