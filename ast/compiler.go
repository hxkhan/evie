package ast

import (
	"fmt"

	"github.com/hk-32/evie/ast/scope"
	"github.com/hk-32/evie/core"
)

func NewVM(exports map[string]core.Value, optimise bool) *Machine {
	vm := &Machine{
		globals:              make(map[string]int),
		root:                 scope.NewScope(len(exports)),
		uninitializedGlobals: make(map[string]struct{}),
		optimise:             optimise,
	}
	vm.scope = vm.root

	builtins := make([]core.Value, len(exports))
	for name, value := range exports {
		index, ok := vm.scope.Declare(name)
		if !ok {
			panic("exports contain conflicting names")
		}
		builtins[index] = value
	}

	// extend from builtin to global scope
	vm.scope = vm.scope.New()
	vm.Machine = core.NewMachine(builtins)
	return vm
}

func (vm *Machine) Run(node Node) (core.Value, error) {
	code := node.compile(vm)

	vm.AcquireGIL()
	defer vm.ReleaseGIL()

	// ensure that the globals are large enough
	if len(vm.Globals) < len(vm.globals) {
		for range len(vm.globals) - len(vm.Globals) {
			vm.Globals = append(vm.Globals, vm.Boxes.New())
		}
	}

	// fetch a coroutine and prepare it
	rt := vm.Coroutines.New()
	rt.Stack = vm.Globals
	rt.Basis = []int{0}

	// run code
	v, err := code(rt)
	// free coroutine
	vm.Coroutines.Put(rt)
	// check errors
	if err == core.ErrReturnSignal {
		return v, nil
	}
	return core.Value{}, nil
}

func (vm *Machine) GetGlobal(name string) *core.Value {
	if index, exists := vm.globals[name]; exists {
		return vm.Globals[index]
	}
	return nil
}

type Package struct {
	Name    string
	Imports []string
	Code    []Node
}

/*
Hoisting rules etc.
All symbols are first symbolically pre-declared without initialization.
This is so when we later initialize them; they can reference each other.
Then function initializations are physically moved to the top of the code.
And finally the rest of the code follows right after.

So this is not possible because the order is maintained:

	x := y + 2
	y := 10

But this is; becuase the declaration ends up being shifted to the top:

	x := 10
	echo x
*/
func (p Package) compile(vm *Machine) core.Instruction {
	var code []core.Instruction

	// 1. declare all symbols
	for _, node := range p.Code {
		if fnDec, isFnDecl := node.(Fn); isFnDecl {
			idx, _ := vm.scope.Declare(fnDec.Name)
			vm.globals[fnDec.Name] = idx
		}

		if iDec, isIdentDec := node.(IdentDec); isIdentDec {
			vm.uninitializedGlobals[iDec.Name] = struct{}{}
			idx, _ := vm.scope.Declare(iDec.Name)
			vm.globals[iDec.Name] = idx
		}
	}

	// 2. physically move function initialization to the top
	for _, node := range p.Code {
		if fnDec, isFnDecl := node.(Fn); isFnDecl {
			code = append(code, fnDec.compileInGlobal(vm, vm.globals[fnDec.Name]))
		}
	}

	// compile the rest of the code
	for _, node := range p.Code {
		if _, isFnDecl := node.(Fn); isFnDecl {
			continue
		}

		// compile global variable declarations in a special way
		if iDec, isIdentDec := node.(IdentDec); isIdentDec {
			code = append(code, iDec.compileInGlobal(vm, vm.globals[iDec.Name]))
			delete(vm.uninitializedGlobals, iDec.Name)
			continue
		}

		// other code
		in := node.compile(vm)
		code = append(code, in)
	}

	return func(rt *core.CoRoutine) (core.Value, error) {
		for _, in := range code {
			if v, err := in(rt); err != nil {
				return v, err
			}
		}
		return core.Value{}, nil
	}
}

type Machine struct {
	core.Machine // embed runtime machine

	globals                    map[string]int
	openFunctionsRefs          [][]core.Reference // open functions and their captured variables
	openFunctionsEscapedLocals []map[int]struct{} // open functions and their escapee locals

	scope                *scope.Instance // current scope
	root                 *scope.Instance // built-in scope
	uninitializedGlobals map[string]struct{}
	optimise             bool
}

// reach searches for a binding across all scope instances
func (vm *Machine) reach(name string) (ref core.Reference, err error) {
	for scope, scroll := range vm.scope.Instances() {
		if index, success := scope.Reach(name); success {
			// if built-in scope then return negative scroll to signal that
			if scope.Previous() == nil {
				return core.Reference{Index: index, Scroll: -1}, nil
			}

			// if accessing global from global; make sure it is initialized
			if scope.Previous() == vm.root && scroll == 0 {
				if _, has := vm.uninitializedGlobals[name]; has {
					return ref, fmt.Errorf("vm.reach(\"%v\") -> unitialized symbol", name)
				}
			}
			return core.Reference{Index: index, Scroll: scroll}, nil
		}
	}
	return ref, fmt.Errorf("vm.reach(\"%v\") -> unreachable symbol", name)
}

func (vm *Machine) addToCaptured(ref core.Reference) (index int) {
	accessingGlobal := len(vm.openFunctionsEscapedLocals) == ref.Scroll
	if !accessingGlobal {
		// owner of variable needs to know that its local has escaped
		escapedLocals := vm.openFunctionsEscapedLocals[len(vm.openFunctionsEscapedLocals)-1-ref.Scroll]
		escapedLocals[ref.Index] = struct{}{}
	}

	// capture the ref if not already captured
	ourRefs := vm.openFunctionsRefs[len(vm.openFunctionsRefs)-1]
	for i, theRef := range ourRefs {
		if theRef == ref {
			return i
		}
	}
	ourRefs = append(ourRefs, ref)
	vm.openFunctionsRefs[len(vm.openFunctionsRefs)-1] = ourRefs
	return len(ourRefs) - 1
}

func (vm *Machine) openFunction() {
	vm.openFunctionsRefs = append(vm.openFunctionsRefs, nil)
	vm.openFunctionsEscapedLocals = append(vm.openFunctionsEscapedLocals, map[int]struct{}{})
}

func (vm *Machine) closeFunction() (refs []core.Reference, escapees map[int]struct{}) {
	// pop top
	refs = vm.openFunctionsRefs[len(vm.openFunctionsRefs)-1]
	vm.openFunctionsRefs = vm.openFunctionsRefs[:len(vm.openFunctionsRefs)-1]

	escapees = vm.openFunctionsEscapedLocals[len(vm.openFunctionsEscapedLocals)-1]
	vm.openFunctionsEscapedLocals = vm.openFunctionsEscapedLocals[:len(vm.openFunctionsEscapedLocals)-1]

	return refs, escapees
}
