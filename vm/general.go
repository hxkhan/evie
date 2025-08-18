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
	"github.com/hxkhan/evie/vm/fields"
)

type Instance struct {
	cp  compiler
	rt  runtime
	log logger
}

type closure struct {
	captures ds.Slice[capture]
	freeVars ds.Set[int]
	scope    ds.Scope
	this     *Value
}

type compiler struct {
	inline  bool              // use dispatch inlining (combining instructions into one)
	statics map[string]*Value // implicitly available to all user packages

	pkg      *packageInstance   // the package being compiled right now
	closures ds.Slice[*closure] // currently open closures

	resolver func(name string) Package
}

type runtime struct {
	packages map[string]*packageInstance // loaded packages
	fibers   sync.Pool                   // pooled fibers for this vm
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
	name    string               // name of the package
	globals map[fields.ID]Global // all global symbols
}

// PackageContructor returns a new instance of a host package
type PackageContructor func() map[string]*Value

type Options struct {
	LogCache        bool // log cache hits/misses
	LogCaptures     bool // log when and what is captured
	DisableInlining bool // use dispatch inlining (combining instructions into one)
	Metrics         bool // collect metrics (affects performance)
	TopLevelLogic   bool // whether to only allow declarations at top level

	ImportsResolver  func(name string) Package // to instantiate host packages when user packages import them
	UniversalStatics map[string]*Value         // implicitly visible to all user packages
}

func New(opts Options) (vm *Instance) {
	vm = &Instance{
		compiler{
			resolver: opts.ImportsResolver,
			statics:  opts.UniversalStatics,
			inline:   !opts.DisableInlining,
		},
		runtime{
			packages: make(map[string]*packageInstance),
		},
		logger{
			Logger:      *log.New(os.Stdout, "", 0),
			logCaptures: opts.LogCaptures,
		},
	}

	vm.rt.fibers = sync.Pool{
		New: func() any {
			return &fiber{vm: vm, boxes: make([]Value, 48)}
		},
	}

	return vm
}

func (vm *Instance) EvalNode(node ast.Node) (result Value, err error) {
	vm.rt.AcquireGIL()
	defer vm.rt.ReleaseGIL()

	if pkg, isPackage := node.(ast.Package); isPackage {
		v, exc := vm.runPackage(pkg)
		if exc != nil {
			err = exc
		}
		result = v
	} else {
		fbr := vm.rt.fibers.Get().(*fiber)
		fbr.unsynchronized = false
		fbr.active = &UserFn{funcInfoStatic: &funcInfoStatic{name: "anonymous"}}
		fbr.base = 0
		fbr.stack = fbr.stack[:0]

		v, exc := vm.compile(node)(fbr)
		if exc != nil {
			err = exc
		}
		result = v
	}

	// don't implicitly return the return value of the last executed instruction
	switch err {
	case nil:
		return result, nil
	case returnSignal:
		return result, nil
	default:
		return result, err
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
	index := fields.Get(name)
	ref, exists := pkg.globals[index]
	if exists {
		*(ref.Value) = value
	} else {
		pkg.globals[index] = Global{Value: &value, IsPublic: true, IsStatic: true}
	}
	return exists
}

func (pkg *packageInstance) GetSymbol(name string) (value Global, exists bool) {
	index := fields.Get(name)
	v, exists := pkg.globals[index]
	return v, exists
}

func (pkg *packageInstance) HasSymbol(name string) (exists bool) {
	_, exists = pkg.globals[fields.Get(name)]
	return exists
}

func (vm *Instance) WaitForNoActivity() {
	vm.rt.wg.Wait()
}

type local struct {
	index      int16
	isCaptured bool
	isStatic   bool
}

// reach searches for a symbol across all scopes
func (cp *compiler) reach(name string) (v any, err error) {
	// 1. check stack
	for scroll := range cp.closures.Len() {
		closure := cp.closures.Last(scroll)
		if binding, success := closure.scope.Reach(name); success {
			// check if it is a local
			if scroll == 0 {
				return local{index: int16(binding.Index), isCaptured: false, isStatic: binding.IsStatic}, nil
			}

			// otherwise capture it & return the index
			return local{index: int16(cp.addToCaptured(scroll, binding.Index)), isCaptured: true, isStatic: binding.IsStatic}, nil
		}
	}

	// 2. check package globals
	if ref, exists := cp.pkg.globals[fields.Get(name)]; exists {
		return ref, nil
	}

	// 3. check universal statics
	if value, exists := cp.statics[name]; exists {
		// wrap as a global
		return Global{Value: value, IsPublic: true, IsStatic: true}, nil
	}

	// 4. check builtins
	if value, exists := builtins[name]; exists {
		// wrap as a global
		return Global{Value: value, IsPublic: true, IsStatic: true}, nil
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

func (rt *runtime) AcquireGIL() {
	rt.gil.Lock()
	//fmt.Println("Someone acquired the GIL")
}

func (rt *runtime) ReleaseGIL() {
	rt.gil.Unlock()
	//fmt.Println("Someone released the GIL")
}
