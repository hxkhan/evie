package vm

import (
	"errors"
	"fmt"
	"iter"
	"log"
	"os"
	"slices"
	"sync"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
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
	ustruct  *userStruct
}

// PackageResolver resolves a package by import name or path.
type PackageResolver func(name string) (*Package, error)

type compiler struct {
	inline  bool         // use dispatch inlining (combining instructions into one)
	prelude *Package     // symbols visible to every package without an import
	src     *diag.Source // the source being compiled, for error positions

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

// pkgGlobal is a package-level binding
type pkgGlobal struct {
	*Value
	IsPublic   bool // visible to other packages
	IsStatic   bool // cannot be reassigned
	IsConstant bool // its value is fixed and known while compiling, so uses of it fold
}

type Options struct {
	Resolver PackageResolver // finds a package by import path when it is not loaded yet
	Prelude  *Package        // symbols visible everywhere without an import; the builtin types are added to it

	LogCaptures     bool // log when and what is captured
	DisableInlining bool // compile without the fast paths for constants, locals and known callees; for debugging the compiler
}

func New(opts Options) (vm *Instance) {
	prelude := opts.Prelude
	if prelude == nil {
		prelude = NewPackage("prelude")
	}
	for name, value := range builtins {
		if !prelude.Has(name) {
			prelude.Set(name, *value)
		}
	}

	vm = &Instance{
		compiler{
			resolver: opts.Resolver,
			prelude:  prelude,
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

// newFiber hands out a fiber for calling into the instance from Go. Give it
// back with ReleaseFiber once the calls are done, or pass nil to Call and let
// the instance borrow one itself.
func (vm *Instance) newFiber() (fbr *fiber) {
	fbr = vm.rt.fibers.Get().(*fiber)
	fbr.reset()
	return fbr
}

// releaseFiber returns a fiber obtained from NewFiber to the pool
func (vm *Instance) releaseFiber(fbr *fiber) {
	fbr.reset()
	vm.rt.fibers.Put(fbr)
}

func (vm *Instance) evalNode(node ast.Node) (result Value, err error) {
	vm.rt.acquire()
	defer vm.rt.release()

	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*CompileError); ok {
				err = ce
			} else if exc, ok := r.(exception); ok {
				err = exc
			} else if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
		}
	}()

	if pkg, isPackage := node.(ast.Package); isPackage {
		src := vm.cp.src
		if src == nil {
			src = diag.NewSource("", nil)
		}
		v, exc := vm.runPackage([]unit{{src: src, node: pkg}})
		if exc != nil {
			err = exc
		}
		result = v
	} else {
		fbr := vm.newFiber()
		defer vm.releaseFiber(fbr)
		fbr.running = true // the runner is held above
		fbr.active = &userFn{funcInfoStatic: &funcInfoStatic{name: "anonymous"}}

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

// EvalSource parses and evaluates a named source; errors show its text
func (vm *Instance) EvalSource(src *diag.Source) (Value, error) {
	node, err := parser.ParseSource(src)
	if err != nil {
		return Value{}, err
	}

	prev := vm.cp.src
	vm.cp.src = src
	defer func() { vm.cp.src = prev }()
	return vm.evalNode(node)
}

// EvalPackage compiles one package from all of its source files and runs its
// initializers. The files are compiled together, so any of them can use what
// another declares; each must declare the same package.
func (vm *Instance) EvalPackage(sources ...*diag.Source) (*Package, error) {
	if len(sources) == 0 {
		return nil, errors.New("EvalPackage needs at least one source")
	}
	units := make([]unit, len(sources))
	for i, src := range sources {
		node, err := parser.ParseSource(src)
		if err != nil {
			return nil, err
		}
		pkg, ok := node.(ast.Package)
		if !ok {
			return nil, fmt.Errorf("%s does not declare a package", src.Name)
		}
		units[i] = unit{src: src, node: pkg}
	}
	if err := vm.evalPackageUnits(units); err != nil {
		return nil, err
	}
	return vm.rt.packages[units[0].node.Name], nil
}

// evalPackageUnits runs runPackage under the runner, turning panics into errors
func (vm *Instance) evalPackageUnits(units []unit) (err error) {
	vm.rt.acquire()
	defer vm.rt.release()

	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*CompileError); ok {
				err = ce
			} else if exc, ok := r.(exception); ok {
				err = exc
			} else if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
		}
	}()

	if _, exc := vm.runPackage(units); exc != nil {
		return exc
	}
	return nil
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

// Package retrieves a loaded package, if not found; then nil is returned
func (vm *Instance) Package(name string) (pkg *Package) {
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

// operand is what the compiler knows about an expression without running it:
// a Value when it folds to a constant, a local when it names a stack slot, or
// a Global when it names a package binding
type operand interface {
	operand()
}

// binding is an operand that names a variable: a local or a Global
type binding interface {
	operand
	binding()
}

func (Value) operand()     {}
func (local) operand()     {}
func (local) binding()     {}
func (pkgGlobal) binding() {}

// reach searches for a symbol across all scopes
func (cp *compiler) reach(name string) (binding, error) {
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

	// 3. check the prelude
	if ref, exists := cp.prelude.globals[fields.Get(name)]; exists {
		return ref, nil
	}

	ce := compileErrorf("cannot find '%v' in this scope", name)
	if closest, ok := diag.Suggest(name, cp.visibleNames()); ok {
		ce.Hint = fmt.Sprintf("a binding with a similar name exists: '%s'", closest)
	}
	if cp.pkg != nil {
		if imp, ok := cp.pkg.importedAs[name]; ok {
			ce.Hint = fmt.Sprintf("the import \"%s\" is bound as '%s', the name its package declares", imp[0], imp[1])
		}
	}
	return nil, ce
}

// visibleNames lists everything a name could refer to at this point of the compilation
func (cp *compiler) visibleNames() []string {
	var names []string
	for scroll := range cp.closures.Len() {
		names = append(names, cp.closures.Last(scroll).scope.Names()...)
	}
	if cp.pkg != nil {
		for id := range cp.pkg.globals {
			names = append(names, fields.Lookup(id))
		}
	}
	for id := range cp.prelude.globals {
		names = append(names, fields.Lookup(id))
	}
	return names
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
