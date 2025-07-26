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
	pkg      *Package          // the package being compiled right now
	closures ds.Slice[closure] // currently open closures
	scope    *scope.Instance   // current scope
	optimise bool
}

type runtime struct {
	packages map[string]*Package

	boxes  ds.Slice[*Value] // pooled boxes for this vm
	fibers ds.Slice[*fiber] // pooled fibers for this vm
	trace  []string         // call-stack trace
	gil    sync.Mutex       // global interpreter lock
	wg     sync.WaitGroup   // wait for all fibers to complete
}

type Package struct {
	symbols map[string]Symbol
}

type Symbol struct {
	*Value
	IsPublic bool
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
			optimise: opts.Optimise,
		},
		runtime{
			packages: make(map[string]*Package),
			boxes:    make(ds.Slice[*Value], 0, 48), // the capacity to store 48 boxed values
			fibers:   make(ds.Slice[*fiber], 0, 3),  // the capacity to store 3 fibers
		},
		&fiber{
			active: &UserFn{funcInfoStatic: &funcInfoStatic{name: "global"}},
			stack:  make([]*Value, 48),
			base:   0,
		},
	}

	// 1. declare exported builtins
	/* vm.rt.builtins = make([]Value, len(opts.Builtins))
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
	} */

	return vm
}

func (vm *Instance) EvalNode(node ast.Node) (Value, error) {
	code := vm.compile(node)

	vm.rt.gil.Lock()
	defer vm.rt.gil.Unlock()

	// run code
	v, err := code(vm.main)

	// check errors
	if err == errReturnSignal {
		return v, nil
	}
	return Value{}, nil
}

func (vm *Instance) GetPackage(name string) *Package {
	return vm.rt.packages[name]
}

func (pkg *Package) GetGlobal(name string) (sym Symbol, exists bool) {
	sym, exists = pkg.symbols[name]
	return sym, exists
}

func (vm *Instance) WaitForNoActivity() {
	vm.rt.wg.Wait()
}

type local int
type captured int

// reach searches for a binding across all scope instances
func (cp *compiler) reach(name string) (v any, err error) {
	// Check stack
	for scope, scroll := range cp.scope.Instances() {
		if index, success := scope.Reach(name); success {
			// check if it's a local
			if scroll == 0 {
				return local(index), nil
			}

			// otherwise capture it & return the index
			return captured(cp.addToCaptured(scroll, index)), nil
		}
	}

	// Now check globals and built-ins
	if sym, exists := cp.pkg.symbols[name]; exists {
		return sym.Value, nil
	}

	return nil, fmt.Errorf("cp.reach(\"%v\") -> unreachable symbol", name)
}

func (cp *compiler) addToCaptured(scroll int, index int) (idx int) {
	// 1. initial-capture logic
	iCaptureLocal := cp.closures.Len() - scroll
	closure := cp.closures[iCaptureLocal]
	// if not referenced already; reference now, as local
	if index = slices.Index(closure.captures, capture{true, index}); index == -1 {
		index = closure.captures.Len()
		closure.captures.Push(capture{true, index})
		cp.closures[iCaptureLocal] = closure
	}

	// 2. propagate the capture down to where we need it (only if ref.scroll > 1)
	for i := iCaptureLocal + 1; i < cp.closures.Len(); i++ {
		closure := cp.closures[i]
		// if not referenced already; reference now, as captured
		if index = slices.Index(closure.captures, capture{false, index}); index == -1 {
			index = closure.captures.Len()
			closure.captures.Push(capture{false, index})
			cp.closures[i] = closure
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
