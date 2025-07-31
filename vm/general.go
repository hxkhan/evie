package vm

import (
	"fmt"
	"iter"
	"log"
	"os"
	"slices"
	"sync"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/ds"
	"github.com/hxkhan/evie/parser"
)

type Instance struct {
	cp   compiler
	rt   runtime
	main *fiber
	log  logger
}

type closure struct {
	captures ds.Slice[capture]
	freeVars ds.Set[int]
	scope    ds.Scope
	this     *Value
}

type compiler struct {
	inline  bool             // use dispatch inlining (combining instructions into one)
	statics map[string]Value // implicitly available to all user packages

	pkg      *packageInstance   // the package being compiled right now
	closures ds.Slice[*closure] // currently open closures

	resolver func(name string) Package
}

type runtime struct {
	packages map[string]*packageInstance // loaded packages
	boxes    ds.Slice[*Value]            // pooled boxes for this vm
	fibers   ds.Slice[*fiber]            // pooled fibers for this vm
	trace    []string                    // call-stack trace
	gil      sync.Mutex                  // global interpreter lock
	wg       sync.WaitGroup              // wait for all fibers to complete
}

// Global is just a wrapper for a global variable reference
type Global struct {
	*Value
	IsPublic bool
	IsStatic bool
}

type packageInstance struct {
	name    string         // name of the package
	globals map[int]Global // all global symbols
}

// PackageContructor returns a new instance of a host package
type PackageContructor func() map[string]*Value

type Options struct {
	LogCache        bool // log cache hits/misses
	LogCaptures     bool // log when and what is captured
	DisableInlining bool // use dispatch inlining (combining instructions into one)
	Metrics         bool // collect metrics (affects performance)
	TopLevelLogic   bool // whether to only allow declarations at top level

	ImportResolver   func(name string) Package // to instantiate host packages when user packages import them
	UniversalStatics map[string]Value          // implicitly visible to all user packages
}

func New(opts Options) *Instance {

	return &Instance{
		compiler{
			resolver: opts.ImportResolver,
			statics:  opts.UniversalStatics,
			inline:   !opts.DisableInlining,
		},
		runtime{
			packages: make(map[string]*packageInstance),
			boxes:    make(ds.Slice[*Value], 0, 48), // the capacity to store 48 boxed values
			fibers:   make(ds.Slice[*fiber], 0, 3),  // the capacity to store 3 fibers
		},
		&fiber{
			active: &UserFn{funcInfoStatic: &funcInfoStatic{name: "global"}},
			stack:  make([]*Value, 48),
			base:   0,
		},
		logger{
			Logger:      *log.New(os.Stdout, "", 0),
			logCaptures: opts.LogCaptures,
		},
	}
}

func (vm *Instance) EvalNode(node ast.Node) (Value, error) {
	code := vm.compile(node)

	vm.rt.gil.Lock()
	defer vm.rt.gil.Unlock()

	// run code
	v, exc := code(vm.main)

	// don't implicitly return the return value of the last executed instruction
	switch exc {
	case nil:
		return v, nil
	case returnSignal:
		return v, nil
	default:
		return v, exc
	}
}

func (vm *Instance) EvalScript(input []byte) (Value, error) {
	output, err := parser.Parse(input)
	if err != nil {
		return Value{}, err
	}

	return vm.EvalNode(output)
}

// Packages iterates through loaded packages
func (vm *Instance) Packages() iter.Seq[Package] {
	return func(yield func(Package) bool) {
		for _, pkg := range vm.rt.packages {
			if !yield(pkg) {
				return
			}
		}
	}
}

// GetPackage retrieves a loaded package, if not found; then nil is returned
func (vm *Instance) GetPackage(name string) (pkg Package) {
	pkg, exists := vm.rt.packages[name]
	if !exists {
		return nil
	}
	return pkg
}

func (pkg *packageInstance) SetSymbol(name string, value Value) (overridden bool) {
	index := fields.get(name)
	ref, exists := pkg.globals[index]
	if exists {
		*(ref.Value) = value
	} else {
		pkg.globals[index] = Global{Value: &value}
	}
	return exists
}

func (pkg *packageInstance) GetSymbol(name string) (value Global, exists bool) {
	index := fields.get(name)
	value, exists = pkg.globals[index]
	return value, exists
}

func (pkg *packageInstance) HasSymbol(name string) (exists bool) {
	_, exists = pkg.globals[fields.get(name)]
	return exists
}

func (vm *Instance) WaitForNoActivity() {
	vm.rt.wg.Wait()
}

type local int
type captured int

// reach searches for a symbol across all scopes
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
	if ref, exists := cp.pkg.globals[fields.get(name)]; exists {
		return ref, nil
	}

	// 4. check universal statics
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
