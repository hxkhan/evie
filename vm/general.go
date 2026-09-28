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
	info     *funcInfoStatic
	ustruct  *UserStruct
}

// PackageResolver resolves a package by import name or path.
type PackageResolver func(name string) (*Package, error)

type compiler struct {
	inline  bool              // use dispatch inlining (combining instructions into one)
	statics map[string]*Value // implicitly available to all user packages

	pkg      *Package           // the package being compiled right now
	closures ds.Slice[*closure] // currently open closures

	resolver PackageResolver
}

type runtime struct {
	packages map[string]*Package // loaded packages
	fibers   sync.Pool           // pooled fibers for this vm
	runner   sync.Mutex          // held by the fiber that is executing evie code
	wg       sync.WaitGroup      // wait for all fibers to complete
}

// Global is just a wrapper for a global variable reference
type Global struct {
	*Value
	IsPublic bool
	IsStatic bool
}

type Options struct {
	LogCaptures     bool // log when and what is captured
	DisableInlining bool // use dispatch inlining (combining instructions into one)

	ImportsResolver  func(name string) *Package // legacy/simple resolver
	PackageResolver  PackageResolver            // robust package resolver with error return
	UniversalStatics map[string]*Value          // implicitly visible to all user packages
}

func New(opts Options) (vm *Instance) {
	var res PackageResolver
	if opts.PackageResolver != nil {
		res = opts.PackageResolver
	} else if opts.ImportsResolver != nil {
		legacy := opts.ImportsResolver
		res = func(name string) (*Package, error) {
			p := legacy(name)
			if p == nil {
				return nil, fmt.Errorf("package '%v' not found", name)
			}
			return p, nil
		}
	}

	vm = &Instance{
		compiler{
			resolver: res,
			statics:  opts.UniversalStatics,
			inline:   !opts.DisableInlining,
			closures: make(ds.Slice[*closure], 0, 6),
		},
		runtime{
			packages: make(map[string]*Package),
		},
		logger{
			Logger:      *log.New(os.Stdout, "", 0),
			logCaptures: opts.LogCaptures,
		},
	}

	vm.rt.fibers = sync.Pool{
		New: func() any {
			return newFiber(vm)
		},
	}

	return vm
}

// NewFiber hands out a fiber for calling into the instance from Go. Give it
// back with ReleaseFiber once the calls are done, or pass nil to Call and let
// the instance borrow one itself.
func (vm *Instance) NewFiber() (fbr *Fiber) {
	fbr = vm.rt.fibers.Get().(*Fiber)
	fbr.reset()
	return fbr
}

// ReleaseFiber returns a fiber obtained from NewFiber to the pool
func (vm *Instance) ReleaseFiber(fbr *Fiber) {
	fbr.reset()
	vm.rt.fibers.Put(fbr)
}

func (vm *Instance) EvalNode(node ast.Node) (result Value, err error) {
	vm.rt.acquire()
	defer vm.rt.release()

	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*CompileError); ok {
				err = ce
			} else if exc, ok := r.(Exception); ok {
				err = exc
			} else if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
		}
	}()

	if pkg, isPackage := node.(ast.Package); isPackage {
		v, exc := vm.runPackage(pkg)
		if exc != nil {
			err = exc
		}
		result = v
	} else {
		fbr := vm.NewFiber()
		defer vm.ReleaseFiber(fbr)
		fbr.running = true // the runner is held above
		fbr.active = &UserFn{funcInfoStatic: &funcInfoStatic{name: "anonymous"}}

		hasClosure := vm.cp.closures.Len() > 0
		if !hasClosure {
			vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: fbr.active.funcInfoStatic})
			vm.cp.closures.Last(0).scope.OpenBlock()
			defer func() {
				if vm.cp.closures.Len() > 0 {
					vm.cp.closures.Pop()
				}
			}()
		}

		v, exc := fbr.runGuarded(vm.compile(node))
		if exc != nil {
			err = exc
		}
		result = v
	}

	// don't implicitly return the return value of the last executed instruction
	switch err {
	case nil:
		return result, nil
	case signalReturn:
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
func (vm *Instance) Packages() iter.Seq[*Package] {
	return func(yield func(*Package) bool) {
		for _, pkg := range vm.rt.packages {
			if !yield(pkg) {
				return
			}
		}
	}
}

// GetPackage retrieves a loaded package, if not found; then nil is returned
func (vm *Instance) GetPackage(name string) (pkg *Package) {
	pkg, exists := vm.rt.packages[name]
	if !exists {
		return nil
	}
	return pkg
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
	if cp.pkg != nil {
		if ref, exists := cp.pkg.globals[fields.Get(name)]; exists {
			return ref, nil
		}
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

func (rt *runtime) acquire() {
	rt.runner.Lock()
}

func (rt *runtime) release() {
	rt.runner.Unlock()
}
