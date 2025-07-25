package vm

import (
	"fmt"
	"slices"
	"sync"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/ds"
	"github.com/hxkhan/evie/vm/scope"
)

type Instance struct {
	opts Options
	cp   compiler
	rt   runtime
	main *fiber
}

type closure struct {
	captures ds.Slice[capture]
	freeVars ds.Set[int]
}

type compiler struct {
	globals map[string]int // maps global variables to their indices

	closures             ds.Slice[closure] // currently open closures
	scope                *scope.Instance   // current scope
	root                 *scope.Instance   // built-in scope
	uninitializedGlobals ds.Set[int]
	optimise             bool
}

type runtime struct {
	builtins []Value          // can get from but can't set in
	boxes    ds.Slice[*Value] // pooled boxes for this vm
	fibers   ds.Slice[*fiber] // pooled fibers for this vm
	trace    []string         // call-stack trace
	gil      sync.Mutex       // global interpreter lock
	wg       sync.WaitGroup   // wait for all fibers to complete
}

type Options struct {
	Optimise      bool             // use specialised instructions
	ObserveIt     bool             // collect metrics (affects performance)
	TopLevelLogic bool             // whether to only allow declarations at top level
	Builtins      map[string]Value // what should be made available to the user in the built-in scope
	Globals       map[string]Value // what should be made available to the user in the global scope
}

func New(opts Options) *Instance {
	vm := &Instance{
		opts,
		compiler{
			globals:              make(map[string]int),
			root:                 scope.NewScope(len(opts.Builtins)),
			uninitializedGlobals: ds.Set[int]{},
			optimise:             opts.Optimise,
		},
		runtime{
			boxes:  make(ds.Slice[*Value], 0, 48), // the capacity to store 48 boxed values
			fibers: make(ds.Slice[*fiber], 0, 3),  // the capacity to store 3 fibers
		},
		&fiber{
			active: &UserFn{funcInfoStatic: &funcInfoStatic{name: "global"}},
			stack:  make([]*Value, 48),
			base:   0,
		},
	}

	// 1. declare exported builtins
	vm.rt.builtins = make([]Value, len(opts.Builtins))
	for name, value := range opts.Builtins {
		index, _ := vm.cp.root.Declare(name)
		vm.rt.builtins[index] = value
	}

	// 2. extend from builtin to global scope
	vm.cp.scope = vm.cp.root.New(len(opts.Globals))

	// 3. declare exported globals
	vm.main.stack = make([]*Value, len(opts.Globals))
	for name, value := range opts.Globals {
		index, _ := vm.cp.scope.Declare(name)
		vm.main.stack[index] = &value
	}

	return vm
}

func (vm *Instance) EvalNode(node ast.Node) (Value, error) {
	code := vm.compile(node)

	vm.rt.gil.Lock()
	defer vm.rt.gil.Unlock()

	// check if more globals have been declared
	if len(vm.main.stack) < len(vm.cp.globals) {
		for range len(vm.cp.globals) - len(vm.main.stack) {
			vm.main.stack = append(vm.main.stack, vm.newValue())
		}
	}

	// run code
	v, err := code(vm.main)

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
func (cp *compiler) reach(name string) (ref reference, err error) {
	for scope, scroll := range cp.scope.Instances() {
		if index, success := scope.Reach(name); success {
			// if built-in scope then return negative scroll to signal that
			if scope.Previous() == nil {
				return reference{index: index, scroll: -1}, nil
			}

			// if accessing global from global; make sure it is initialized
			if scope.Previous() == cp.root && scroll == 0 {
				if cp.uninitializedGlobals.Has(index) {
					return ref, fmt.Errorf("cp.reach(\"%v\") -> unitialized symbol", name)
				}
			}
			return reference{index: index, scroll: scroll}, nil
		}
	}
	return ref, fmt.Errorf("cp.reach(\"%v\") -> unreachable symbol", name)
}

func (cp *compiler) addToCaptured(ref reference) (index int) {
	// are we scrolling to global scope?
	accessingGlobal := cp.closures.Len() == ref.scroll

	// 1. initial-capture logic
	iCapture := cp.closures.Len() - ref.scroll
	closure := cp.closures[iCapture]
	// if not referenced already; reference now, as local
	if index = slices.Index(closure.captures, capture{true, ref.index}); index == -1 {
		index = closure.captures.Len()
		closure.captures.Push(capture{true, ref.index})
		cp.closures[iCapture] = closure

		// owner of variable needs to know that its local has escaped so it does not recycle it
		if !accessingGlobal {
			owner := cp.closures[iCapture-1]
			owner.freeVars.Add(ref.index)
		}
	}

	// 2. propagate the capture down to where we need it (only if ref.scroll > 1)
	for i := range ref.scroll - 1 {
		current := iCapture + 1 + i
		closure := cp.closures[current]
		// if not referenced already; reference now, as captured
		if index = slices.Index(closure.captures, capture{false, index}); index == -1 {
			index = closure.captures.Len()
			closure.captures.Push(capture{false, index})
			cp.closures[current] = closure
		}
	}
	return index
}

func (cp *compiler) openClosure() {
	cp.closures.Push(closure{freeVars: ds.Set[int]{}})
}

func (cp *compiler) closeClosure() closure {
	return cp.closures.Pop()
}

func (vm *Instance) newValue() (obj *Value) {
	if vm.rt.boxes.IsEmpty() {
		return new(Value)
	}
	return vm.rt.boxes.Pop()
}

func (vm *Instance) putValue(obj *Value) {
	if vm.rt.boxes.Len() < vm.rt.boxes.Cap() {
		vm.rt.boxes.Push(obj)
	}
}

func (vm *Instance) newFiber() (obj *fiber) {
	if vm.rt.fibers.IsEmpty() {
		return new(fiber)
	}
	return vm.rt.fibers.Pop()
}

func (vm *Instance) putFiber(obj *fiber) {
	if vm.rt.fibers.Len() < vm.rt.fibers.Cap() {
		vm.rt.fibers.Push(obj)
	}
}
