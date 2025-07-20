package vm

import (
	"fmt"
	"slices"
	"sync"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/pool"
	"github.com/hxkhan/evie/vm/scope"
)

type Instance struct {
	cp   compiler
	rt   runtime
	main *fiber
}

type closure struct {
	captures []capture
	freeVars map[int]struct{}
}

type compiler struct {
	globals map[string]int // maps global variables to their indices

	closures slice[closure]

	scope                *scope.Instance // current scope
	root                 *scope.Instance // built-in scope
	uninitializedGlobals map[string]struct{}
	optimise             bool
}

type runtime struct {
	builtins []Value              // can get from but can't set in
	boxes    pool.Instance[Value] // pooled boxes for this vm
	fibers   pool.Instance[fiber] // pooled fibers for this vm
	trace    []string             // call-stack trace
	gil      sync.Mutex           // global interpreter lock
	wg       sync.WaitGroup       // wait for all fibers to complete
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
		&fiber{
			active: &UserFn{funcInfoStatic: &funcInfoStatic{name: "global"}},
		},
	}

	vm.rt.builtins = make([]Value, len(exports))
	for name, value := range exports {
		index, ok := vm.cp.root.Declare(name)
		if !ok {
			panic("exports contain conflicting names")
		}
		vm.rt.builtins[index] = value
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
				if _, has := cp.uninitializedGlobals[name]; has {
					return ref, fmt.Errorf("cp.reach(\"%v\") -> unitialized symbol", name)
				}
			}
			return reference{index: index, scroll: scroll}, nil
		}
	}
	return ref, fmt.Errorf("cp.reach(\"%v\") -> unreachable symbol", name)
}

func (cp *compiler) addToCaptured(ref reference) (index int) {
	accessingGlobal := cp.closures.len() == ref.scroll
	if !accessingGlobal {
		// owner of variable needs to know that its local has escaped
		owner := cp.closures.get(cp.closures.len() - ref.scroll - 1)
		owner.freeVars[ref.index] = struct{}{}
	}

	// initial-capturer logic
	start := cp.closures.len() - ref.scroll
	closure := cp.closures.get(start)
	// if not referenced already; reference now, as local
	if index = slices.Index(closure.captures, capture{true, ref.index}); index == -1 {
		index = len(closure.captures)
		closure.captures = append(closure.captures, capture{true, ref.index})
		cp.closures.set(start, closure)
	}

	for i := range ref.scroll - 1 {
		currentCapturer := start + 1 + i
		owner := cp.closures.get(currentCapturer)
		// if not referenced already; reference now, as captured
		if index = slices.Index(owner.captures, capture{false, index}); index == -1 {
			index = len(owner.captures)
			owner.captures = append(owner.captures, capture{false, index})
			cp.closures.set(currentCapturer, owner)
		}
	}
	return index
}

func (cp *compiler) openFunction() {
	cp.closures.push(closure{freeVars: map[int]struct{}{}})
}

func (cp *compiler) closeFunction() (refs []capture, escapees map[int]struct{}) {
	closure := cp.closures.pop()
	return closure.captures, closure.freeVars
}
