package vm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"os"
	"slices"
	"sync"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/internal/ds"
	"github.com/hxkhan/evie/internal/fields"
	"github.com/hxkhan/evie/parser"
)

// Instance compiles and runs Evie code: it holds the loaded packages and the
// fibers running them. Make one with New.
type Instance struct {
	cp compiler
	rt runtime
}

type closure struct {
	captures      ds.Slice[capture]
	freeVars      ds.Set[int]
	capturedNames map[string]bool // identifier names referenced inside nested functions/classes
	scope         ds.Scope
	info          *funcInfoStatic
	uclass        *userClass
	loops         int
	switches      int
	decls         map[int]nameDecl // slot -> where it is declared, when names are recorded
}

// mayEscape reports whether a local binding with name may be captured by a
// nested closure or class method inside this function.
func (cl *closure) mayEscape(name string) bool {
	return cl.capturedNames[name]
}

// PackageResolver resolves a package by import name or path.
type PackageResolver func(name string) (*Package, error)

type compiler struct {
	inline    bool          // use dispatch inlining (combining instructions into one)
	debugInfo bool          // record slot names and nested closures for Dump
	prelude   *Package      // symbols visible to every package without an import
	registry  *sync.RWMutex // the runtime's registry lock, which also guards the prelude
	src       *diag.Source  // the source being compiled, for error positions

	pkg      *Package           // the package being compiled right now
	closures ds.Slice[*closure] // currently open closures

	resolver PackageResolver
	onName   func(NameEvent)  // receives declared and resolved names; nil unless a tool asks
	errs     *[]*CompileError // the compile errors of the package being compiled, which tolerate collects
}

type runtime struct {
	packages     map[string]*Package // loaded packages by canonical module ID
	byName       map[string]*Package // loaded packages by declared short name (for Lookup/Call)
	packageOrder []string            // compiled Evie module IDs in load order
	scriptInfo   *funcInfoStatic     // last compiled bare script when Options.DebugInfo is set
	fibers       sync.Pool           // pooled fibers for this vm
	runner       sync.Mutex          // held by the fiber that is executing evie code
	wg           sync.WaitGroup      // wait for all fibers to complete
	registry     sync.RWMutex        // guards packages, byName, packageOrder and the prelude's symbols, which hosts read while code loads
	stdout       io.Writer           // where echo writes
	stops                            // when running code has to stop
}

// pkgGlobal is a package-level binding
type pkgGlobal struct {
	*Value
	IsPublic   bool // visible to other packages
	IsStatic   bool // cannot be reassigned
	IsConstant bool // its value is fixed and known while compiling, so uses of it fold
}

// Options configures a new Instance
type Options struct {
	Resolver PackageResolver // finds a package by import path when it is not loaded yet
	Prelude  *Package        // symbols visible everywhere without an import; the builtin types are added to it
	Stdout   io.Writer       // where echo writes; defaults to os.Stdout

	DebugInfo       bool // record slot names, local classes and nested closures during compilation for Dump
	DisableInlining bool // compile without the fast paths for constants, locals and known callees; for debugging the compiler

	// OnName, when set, receives every name the compiler declares or resolves,
	// with where it is declared, for tools such as editors. It is called while
	// code compiles, under the instance's runner.
	OnName func(NameEvent)
}

// New makes an Instance configured by opts
func New(opts Options) (vm *Instance) {
	var prelude *Package
	if opts.Prelude == nil {
		prelude = NewPackage("prelude")
	} else {
		prelude = opts.Prelude.Clone()
	}
	for name, value := range builtins {
		if !prelude.Has(name) {
			prelude.Set(name, *value)
		}
	}

	vm = &Instance{
		cp: compiler{
			resolver:  opts.Resolver,
			prelude:   prelude,
			inline:    !opts.DisableInlining,
			debugInfo: opts.DebugInfo,
			closures:  make(ds.Slice[*closure], 0, 6),
			onName:    opts.OnName,
		},
		rt: runtime{
			packages: make(map[string]*Package),
			byName:   make(map[string]*Package),
			stdout:   opts.Stdout,
		},
	}

	vm.cp.registry = &vm.rt.registry
	if vm.rt.stdout == nil {
		vm.rt.stdout = os.Stdout
	}
	vm.rt.fibers = sync.Pool{
		New: func() any {
			return newFiber(vm)
		},
	}
	vm.rt.stops.init()

	return vm
}

// newFiber hands out a fiber for a call from the host without a context;
// Interrupt stops it. Give it back with releaseFiber.
func (vm *Instance) newFiber() (fbr *fiber) {
	return vm.borrowFiber(vm.rt.current.Load())
}

// borrowFiber hands out a fiber that runs under the stopper s
func (vm *Instance) borrowFiber(s *stopper) (fbr *fiber) {
	s.hold()
	return vm.takeFiber(s)
}

// takeFiber is borrowFiber for a caller that already holds a use of s for
// the fiber, as a parent does for the task it spawns
func (vm *Instance) takeFiber(s *stopper) (fbr *fiber) {
	fbr = vm.rt.fibers.Get().(*fiber) // pooled fibers were reset by releaseFiber
	fbr.attach(s)
	return fbr
}

// releaseFiber returns a borrowed fiber to the pool
func (vm *Instance) releaseFiber(fbr *fiber) {
	fbr.reset()
	fbr.detach()
	vm.rt.fibers.Put(fbr)
}

// evalNode runs a parsed source under the runner. Code it runs stops with s,
// or with the stopper of the load in progress when s is nil. src is set as the
// compiler's source only once the runner is held, so concurrent evaluations
// cannot swap it under each other.
func (vm *Instance) evalNode(src *diag.Source, node ast.Node, s *stopper) (result Value, err error) {
	vm.rt.acquire()
	defer vm.rt.release()
	defer vm.rt.loadingUnder(s)()
	if src == nil {
		src = diag.NewSource("", nil)
	}
	prevSrc := vm.cp.src
	vm.cp.src = src
	defer func() { vm.cp.src = prevSrc }()

	defer func() {
		if r := recover(); r != nil {
			err = loadError(r)
		}
	}()

	if pkg, isPackage := node.(ast.Package); isPackage {
		v, exc := vm.runPackage("", []unit{{src: src, node: pkg}}, true)
		if exc != nil {
			return v, exc
		}
		return Value{}, nil
	}

	fbr := vm.borrowFiber(vm.rt.forLoading())
	defer vm.releaseFiber(fbr)
	fbr.running = true // the runner is held above

	info := &funcInfoStatic{name: "anonymous", def: site{src: src}, vm: vm}
	prevClosures := vm.cp.closures.Len()
	cl := &closure{freeVars: ds.Set[int]{}, info: info, capturedNames: findCapturedNames(node)}
	cl.scope.TrackSlots = vm.cp.debugInfo
	vm.cp.closures.Push(cl)
	cl.scope.OpenBlock()
	defer func() {
		for vm.cp.closures.Len() > prevClosures {
			vm.cp.closures.Pop()
		}
	}()

	info.code = vm.compile(node)
	vm.closeFunction(info, cl)
	if vm.cp.debugInfo {
		vm.rt.scriptInfo = info
	}

	fn := &userFn{funcInfoStatic: info}
	v, exc := fbr.runGuarded(func(fbr *fiber) (Value, exception) {
		return fn.call(fbr, nil)
	})
	if exc != nil {
		return v, exc
	}
	return v, nil
}

// ParsedUnit pairs a named source with its already-parsed AST so a pre-parsed
// program (such as an evie.Bundle) can be compiled into multiple VM instances
// without re-reading or re-parsing source files.
type ParsedUnit struct {
	Source *diag.Source
	Node   ast.Node
}

// EvalSource parses and evaluates a named source; errors show its text. A
// source that declares a package is loaded as that package and gives nil; a
// bare script gives its result. When ctx is done, the code stops with an error
// whose cause is ctx's cause, and so do the packages it imports while it loads.
func (vm *Instance) EvalSource(ctx context.Context, src *diag.Source) (Value, error) {
	node, err := parser.ParseSource(src)
	if err != nil {
		return Value{}, err
	}
	return vm.EvalUnit(ctx, ParsedUnit{Source: src, Node: node})
}

// EvalUnit is EvalSource for a source that is already parsed
func (vm *Instance) EvalUnit(ctx context.Context, pu ParsedUnit) (Value, error) {
	s := vm.rt.scope(ctx)
	defer s.drop()
	return vm.evalNode(pu.Source, pu.Node, s)
}

// CompileUnit compiles a parsed bare script without running it, so Dump can
// show it. A unit that declares a package is loaded the way EvalUnit loads it.
func (vm *Instance) CompileUnit(pu ParsedUnit) (err error) {
	if pkg, isPackage := pu.Node.(ast.Package); isPackage {
		_, err = vm.evalNode(pu.Source, pkg, nil)
		return err
	}

	vm.rt.acquire()
	defer vm.rt.release()

	prevSrc := vm.cp.src
	vm.cp.src = pu.Source
	defer func() { vm.cp.src = prevSrc }()

	prevClosures := vm.cp.closures.Len()
	defer func() {
		for vm.cp.closures.Len() > prevClosures {
			vm.cp.closures.Pop()
		}
		if r := recover(); r != nil {
			err = loadError(r)
		}
	}()

	info := &funcInfoStatic{name: "<script>", def: site{src: pu.Source}, vm: vm}
	cl := &closure{freeVars: ds.Set[int]{}, info: info, capturedNames: findCapturedNames(pu.Node)}
	cl.scope.TrackSlots = vm.cp.debugInfo
	vm.cp.closures.Push(cl)
	cl.scope.OpenBlock()

	_ = vm.compile(pu.Node)
	vm.closeFunction(info, cl)
	vm.rt.scriptInfo = info
	return nil
}

// LoadPackage compiles one package from its parsed files and runs its
// initializers. The files are compiled together, so any of them can use what
// another declares; each must declare the same package. moduleID identifies
// the module, such as its canonical file or directory path; when empty, the
// package's declared name is used. When ctx is done, the initializers stop with
// an error whose cause is ctx's cause, and so do those of the packages it imports.
func (vm *Instance) LoadPackage(ctx context.Context, moduleID string, parsed []ParsedUnit) (*Package, error) {
	return vm.loadPackage(ctx, moduleID, parsed, true)
}

// CheckPackage compiles a package from its parsed files the way LoadPackage
// does, and reports the same compile errors, but runs none of its
// initializers: nothing in the package executes. It is for tools, such as
// editors, that check code without running it. The package counts as loaded,
// with its variables unset, so an instance used for checking is not one to run
// code in.
func (vm *Instance) CheckPackage(moduleID string, parsed []ParsedUnit) (*Package, error) {
	return vm.loadPackage(context.Background(), moduleID, parsed, false)
}

func (vm *Instance) loadPackage(ctx context.Context, moduleID string, parsed []ParsedUnit, run bool) (*Package, error) {
	if len(parsed) == 0 {
		return nil, errors.New("LoadPackage needs at least one unit")
	}
	units := make([]unit, len(parsed))
	for i, pu := range parsed {
		pkg, ok := pu.Node.(ast.Package)
		if !ok {
			return nil, fmt.Errorf("%s does not declare a package", pu.Source.Name)
		}
		units[i] = unit{src: pu.Source, node: pkg}
	}
	s := vm.rt.scope(ctx)
	defer s.drop()
	if err := vm.evalPackageUnits(moduleID, units, s, run); err != nil {
		return nil, err
	}
	id := moduleID
	if id == "" {
		id = units[0].node.Name
	}
	return vm.PackageByID(id), nil
}

// evalPackageUnits runs runPackage under the runner, turning panics into errors
func (vm *Instance) evalPackageUnits(moduleID string, units []unit, s *stopper, run bool) (err error) {
	vm.rt.acquire()
	defer vm.rt.release()
	defer vm.rt.loadingUnder(s)()

	defer func() {
		if r := recover(); r != nil {
			err = loadError(r)
		}
	}()

	if _, exc := vm.runPackage(moduleID, units, run); exc != nil {
		return exc
	}
	return nil
}

// Packages iterates through loaded packages
func (vm *Instance) Packages() iter.Seq[*Package] {
	return func(yield func(*Package) bool) {
		vm.rt.registry.RLock()
		pkgs := slices.Collect(maps.Values(vm.rt.packages))
		vm.rt.registry.RUnlock()
		for _, pkg := range pkgs {
			if !yield(pkg) {
				return
			}
		}
	}
}

// PackageByID retrieves a loaded package by its exact canonical module ID.
func (vm *Instance) PackageByID(id string) *Package {
	vm.rt.registry.RLock()
	defer vm.rt.registry.RUnlock()
	return vm.rt.packages[id]
}

// Package retrieves a loaded package by canonical module ID or declared short
// name; returns nil if not found.
func (vm *Instance) Package(key string) *Package {
	vm.rt.registry.RLock()
	defer vm.rt.registry.RUnlock()
	if pkg, exists := vm.rt.packages[key]; exists {
		return pkg
	}
	if pkg, exists := vm.rt.byName[key]; exists {
		return pkg
	}
	return nil
}

// Wait blocks until every task the instance's code started has finished
func (vm *Instance) Wait() {
	vm.rt.wg.Wait()
}

type local struct {
	index      int16
	isCaptured bool
	isBoxed    bool // true when this local escapes into a nested closure and lives in a heap cell
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

// packageNamed returns the package that name refers to when it names one
// bound in the package being compiled or in the prelude, such as an import,
// rather than a local. Unlike reach it has no effects: it captures nothing and
// reports nothing.
func (cp *compiler) packageNamed(name string) *Package {
	for scroll := range cp.closures.Len() {
		if _, local := cp.closures.Last(scroll).scope.Reach(name); local {
			return nil
		}
	}
	id, known := fields.Find(name)
	if !known {
		return nil
	}
	var g pkgGlobal
	var exists bool
	if cp.pkg != nil {
		g, exists = cp.pkg.globals[id]
	}
	if !exists {
		cp.registry.RLock()
		g, exists = cp.prelude.globals[id]
		cp.registry.RUnlock()
	}
	if !exists || !g.IsConstant || g.Value == nil {
		return nil
	}
	pkg, _ := g.Value.AsPackage()
	return pkg
}

// reach searches for a symbol across all scopes; pos is where the name is
// written, for Options.OnName, and may be unknown
func (cp *compiler) reach(name string, pos diag.Pos) (binding, error) {
	// 1. check stack
	for scroll := range cp.closures.Len() {
		closure := cp.closures.Last(scroll)
		if binding, success := closure.scope.Reach(name); success {
			cp.usedLocal(scroll, binding.Index, name, pos, binding.IsStatic)
			// check if it is a local
			if scroll == 0 {
				return local{index: int16(binding.Index), isCaptured: false, isBoxed: closure.mayEscape(name), isStatic: binding.IsStatic}, nil
			}

			// otherwise capture it & return the index; a captured slot must live in a heap
			// cell, or the closure would read the stack slot's contents as a pointer
			if !closure.mayEscape(name) {
				panic(fmt.Errorf("internal compiler error: '%s' is captured but its slot is not boxed", name))
			}
			return local{index: int16(cp.addToCaptured(scroll, binding.Index)), isCaptured: true, isStatic: binding.IsStatic}, nil
		}
	}

	// 2. check package globals, then 3. the prelude; a name nobody interned is in neither,
	// and looking it up must not intern it
	if id, known := fields.Find(name); known {
		if cp.pkg != nil {
			if ref, exists := cp.pkg.globals[id]; exists {
				cp.usedGlobal(cp.pkg, id, name, pos, ref)
				return ref, nil
			}
		}
		cp.registry.RLock()
		ref, exists := cp.prelude.globals[id]
		cp.registry.RUnlock()
		if exists {
			cp.usedGlobal(nil, id, name, pos, ref)
			return ref, nil
		}
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
	cp.registry.RLock()
	for id := range cp.prelude.globals {
		names = append(names, fields.Lookup(id))
	}
	cp.registry.RUnlock()
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

	// 2. propagate the capture down to where we need it (only if scroll > 1)
	for i := iCaptureLocal + 1; i < cp.closures.Len(); i++ {
		closure := cp.closures[i]
		parentIdx := idx
		// if not referenced already; reference now, as captured from parent
		if idx = slices.Index(closure.captures, capture{false, parentIdx}); idx == -1 {
			idx = closure.captures.Len()
			closure.captures.Push(capture{false, parentIdx})
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
