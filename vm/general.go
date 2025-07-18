package vm

import (
	"fmt"
	"sync"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/pool"
	"github.com/hxkhan/evie/vm/scope"
)

type Instance struct {
	cp   compiler
	rt   runtime
	main fiber
}

type compiler struct {
	builtins []Value        // can get from but can't set in
	globals  map[string]int // maps to global variable addresses

	openFunctionsRefs          [][]reference      // open functions and their captured variables
	openFunctionsEscapedLocals []map[int]struct{} // open functions and their escapee locals

	scope                *scope.Instance // current scope
	root                 *scope.Instance // built-in scope
	uninitializedGlobals map[string]struct{}
	optimise             bool
}

type runtime struct {
	boxes  pool.Instance[Value] // pooled boxes for this vm
	fibers pool.Instance[fiber] // pooled fibers for this vm
	trace  []string             // call-stack trace
	gil    sync.Mutex           // global interpreter lock
	wg     sync.WaitGroup       // wait for all fibers to complete
}

func New(exports map[string]Value, optimise bool) *Instance {
	vm := &Instance{
		compiler{
			globals:              make(map[string]int),
			root:                 scope.NewScope(len(exports)),
			uninitializedGlobals: make(map[string]struct{}),
			optimise:             optimise,
		},
		runtime{
			boxes:  pool.Make[Value](48), // the capacity to store 48 boxed values
			fibers: pool.Make[fiber](3),  // the capacity to store 3 fibers
		},
		fiber{
			basis: []int{0},
		},
	}

	builtins := make([]Value, len(exports))
	for name, value := range exports {
		index, ok := vm.cp.root.Declare(name)
		if !ok {
			panic("exports contain conflicting names")
		}
		builtins[index] = value
	}

	// extend from builtin to global scope
	vm.cp.scope = vm.cp.root.New()
	return vm
}

func (vm *Instance) EvalNode(node ast.Node) (Value, error) {
	code := vm.compile(node)

	vm.rt.gil.Lock()
	defer vm.rt.gil.Unlock()

	// check if more globals have been declared
	if vm.main.stackSize() < len(vm.cp.globals) {
		for range len(vm.cp.globals) - vm.main.stackSize() {
			vm.main.stack = append(vm.main.stack, vm.rt.boxes.Get())
		}
	}

	// run code
	v, err := code(&vm.main)

	// check errors
	if err == errReturnSignal {
		return v, nil
	}
	return Value{}, nil
}

func (vm *Instance) GetGlobal(name string) *Value {
	if index, exists := vm.cp.globals[name]; exists {
		return vm.main.stack[index]
	}
	return nil
}

func (vm *Instance) WaitForNoActivity() {
	vm.rt.wg.Wait()
}

// reach searches for a binding across all scope instances
func (vm *Instance) reach(name string) (ref reference, err error) {
	for scope, scroll := range vm.cp.scope.Instances() {
		if index, success := scope.Reach(name); success {
			// if built-in scope then return negative scroll to signal that
			if scope.Previous() == nil {
				return reference{index: index, scroll: -1}, nil
			}

			// if accessing global from global; make sure it is initialized
			if scope.Previous() == vm.cp.root && scroll == 0 {
				if _, has := vm.cp.uninitializedGlobals[name]; has {
					return ref, fmt.Errorf("vm.reach(\"%v\") -> unitialized symbol", name)
				}
			}
			return reference{index: index, scroll: scroll}, nil
		}
	}
	return ref, fmt.Errorf("vm.reach(\"%v\") -> unreachable symbol", name)
}

func (vm *Instance) addToCaptured(ref reference) (index int) {
	accessingGlobal := len(vm.cp.openFunctionsEscapedLocals) == ref.scroll
	if !accessingGlobal {
		// owner of variable needs to know that its local has escaped
		escapedLocals := vm.cp.openFunctionsEscapedLocals[len(vm.cp.openFunctionsEscapedLocals)-1-ref.scroll]
		escapedLocals[ref.index] = struct{}{}
	}

	// capture the ref if not already captured
	ourRefs := vm.cp.openFunctionsRefs[len(vm.cp.openFunctionsRefs)-1]
	for i, theRef := range ourRefs {
		if theRef == ref {
			return i
		}
	}
	ourRefs = append(ourRefs, ref)
	vm.cp.openFunctionsRefs[len(vm.cp.openFunctionsRefs)-1] = ourRefs
	return len(ourRefs) - 1
}

func (cp *compiler) openFunction() {
	cp.openFunctionsRefs = append(cp.openFunctionsRefs, nil)
	cp.openFunctionsEscapedLocals = append(cp.openFunctionsEscapedLocals, map[int]struct{}{})
}

func (cp *compiler) closeFunction() (refs []reference, escapees map[int]struct{}) {
	// pop top
	refs = cp.openFunctionsRefs[len(cp.openFunctionsRefs)-1]
	cp.openFunctionsRefs = cp.openFunctionsRefs[:len(cp.openFunctionsRefs)-1]

	escapees = cp.openFunctionsEscapedLocals[len(cp.openFunctionsEscapedLocals)-1]
	cp.openFunctionsEscapedLocals = cp.openFunctionsEscapedLocals[:len(cp.openFunctionsEscapedLocals)-1]

	return refs, escapees
}
