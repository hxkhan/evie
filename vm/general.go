package vm

import (
	"fmt"
	"slices"
	"sync"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/ds"
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
	scope    ds.Scope
}

type compiler struct {
	statics  map[string]Value   // implicitly available to all user packages
	pkg      *Package           // the package being compiled right now
	closures ds.Slice[*closure] // currently open closures
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
	name    string
	globals map[string]*Value
	private ds.Set[string]
}

type Symbol struct {
	*Value
	IsPublic bool
}

type Options struct {
	Inline        bool             // use dispatch inlining (combining instructions into one)
	ObserveIt     bool             // collect metrics (affects performance)
	TopLevelLogic bool             // whether to only allow declarations at top level
	Statics       map[string]Value // implicitly available to all user packages
}

func New(opts Options) *Instance {
	return &Instance{
		opts,
		compiler{
			statics: opts.Statics,
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

/* func (vm *Instance) PackageHandle(name string) *Package {
	if pkg, exists := vm.rt.packages[name]; exists {
		return pkg
	}

	pkg := &Package{symbols: make(map[string]Symbol)}

	//return vm.rt.packages[name]
} */

func NewHostPackage(name string, exports map[string]*Value) *Package {
	return &Package{name: name, globals: exports, private: ds.Set[string]{}}
}

func (vm *Instance) GetPackage(name string) *Package {
	return vm.rt.packages[name]
}

func (pkg *Package) GetGlobal(name string) (sym Symbol, exists bool) {
	ref, exists := pkg.globals[name]
	if !exists {
		return sym, false
	}

	_, private := pkg.private[name]
	return Symbol{Value: ref, IsPublic: !private}, exists
}

func (vm *Instance) WaitForNoActivity() {
	vm.rt.wg.Wait()
}

type local int
type captured int

// reach searches for a binding across all scope instances
func (cp *compiler) reach(name string) (v any, err error) {
	// 1. check stack
	for scroll := range cp.closures.Len() {
		closure := cp.closures.Last(scroll)
		if index, success := closure.scope.Reach(name); success {
			// check if it's a local
			if scroll == 0 {
				return local(index), nil
			}

			// otherwise capture it & return the index
			return captured(cp.addToCaptured(scroll, index)), nil
		}
	}

	// 2. check package globals
	if ref, exists := cp.pkg.globals[name]; exists {
		return ref, nil
	}

	// 3. check static builtins
	if value, exists := cp.statics[name]; exists {
		return value, nil
	}

	return nil, fmt.Errorf("cp.reach(\"%v\") -> unreachable symbol", name)
}

func (cp *compiler) addToCaptured(scroll int, index int) (idx int) {
	// 1. initial-capture logic
	iCaptureLocal := cp.closures.Len() - scroll
	closure := cp.closures[iCaptureLocal]
	// if not referenced already; reference now, as local
	if idx = slices.Index(closure.captures, capture{true, index}); idx == -1 {
		idx = closure.captures.Len()
		closure.captures.Push(capture{true, index})

		// owner of variable needs to know that its local has escaped so it does not recycle it
		owner := cp.closures[iCaptureLocal-1]
		owner.freeVars.Add(index)
	}

	// 2. propagate the capture down to where we need it (only if ref.scroll > 1)
	for i := iCaptureLocal + 1; i < cp.closures.Len(); i++ {
		closure := cp.closures[i]
		// if not referenced already; reference now, as captured
		if idx = slices.Index(closure.captures, capture{false, idx}); idx == -1 {
			idx = closure.captures.Len()
			closure.captures.Push(capture{false, idx})
		}
	}
	return idx
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
