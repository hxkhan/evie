package vm

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/internal/ds"
	"github.com/hxkhan/evie/internal/fields"
)

type instruction func(fbr *fiber) (Value, exception)

func (vm *Instance) compile(node ast.Node) instruction {
	switch node := node.(type) {
	case ast.Package:
		panic("cannot compile package here")

	case ast.Input[bool]:
		value := BoxBool(node.Value)
		return func(fbr *fiber) (Value, exception) {
			return value, nil
		}

	case ast.Input[float64]:
		value := BoxNumber(node.Value)
		return func(fbr *fiber) (Value, exception) {
			return value, nil
		}

	case ast.Input[string]:
		value := BoxString(node.Value)
		return func(fbr *fiber) (Value, exception) {
			return value, nil
		}

	case ast.Input[struct{}]:
		return func(fbr *fiber) (Value, exception) {
			return Value{}, nil
		}

	case ast.Break:
		if !vm.canBreak() {
			panic(vm.compileErrorAt(node, "break outside of a loop or switch"))
		}
		return func(fbr *fiber) (Value, exception) {
			return Value{}, signalBreak
		}

	case ast.Continue:
		if !vm.canContinue() {
			panic(vm.compileErrorAt(node, "continue outside of a loop"))
		}
		return func(fbr *fiber) (Value, exception) {
			return Value{}, signalContinue
		}

	case ast.StringTemplate:
		return vm.emitStringTemplate(node)

	case ast.Echo:
		return vm.emitEcho(node)

	case ast.Decl:
		return vm.emitIdentDec(node)

	case ast.Ident:
		return vm.emitIdentGet(node)

	case ast.Assign:
		return vm.emitAssign(node)

	case ast.Block:
		return vm.emitBlock(node)

	case ast.Conditional:
		return vm.emitConditional(node)

	case ast.While:
		return vm.emitWhile(node)

	case ast.For:
		return vm.emitFor(node)

	case ast.Switch:
		return vm.emitSwitch(node)

	case ast.Catch:
		return vm.emitCatch(node)

	case ast.Fn:
		return vm.emitFn(node)

	case ast.Call:
		return vm.emitCall(node)

	case ast.FieldAccess:
		return vm.emitFieldAccess(node)

	case ast.Go:
		return vm.emitGo(node)

	case ast.Return:
		return vm.emitReturn(node)

	case ast.Throw:
		return vm.emitThrow(node)

	case ast.Await:
		return vm.emitAwait(node)

	case ast.AwaitAll:
		return vm.emitAwaitAll(node)

	case ast.AwaitAny:
		return vm.emitAwaitAny(node)

	case ast.AwaitRace:
		return vm.emitAwaitRace(node)

	case ast.AwaitTimeout:
		return vm.emitAwaitTimeout(node)

	case ast.Plus:
		return vm.emitPlus(node)

	case ast.Neg:
		return vm.emitNeg(node)

	case ast.Not:
		return vm.emitNot(node)

	case ast.BinOp:
		return vm.emitBinOp(node)

	case ast.MutableBinOp:
		return vm.emitMutableBinOp(node)

	case ast.IsInstanceOf:
		return vm.emitIsInstanceOf(node)

	case ast.ArrayLiteral:
		return vm.emitArrayLiteral(node)

	case ast.Object:
		return vm.emitObject(node)

	case ast.Exists:
		return vm.emitExists(node)

	case ast.Subscript:
		return vm.emitSubscript(node)

	case ast.Slice:
		return vm.emitSlice(node)

	case ast.ClassDefinition:
		return vm.emitClass(node)
	}

	panic(fmt.Errorf("implement %T", node))
}

// currentClosure returns the innermost closure being compiled, creating a
// fallback anonymous scope when compiling a top-level bare statement.
func (vm *Instance) currentClosure() *closure {
	if vm.cp.closures.Len() == 0 {
		cl := &closure{freeVars: ds.Set[int]{}, info: &funcInfoStatic{name: "anonymous"}}
		cl.scope.TrackSlots = vm.cp.debugInfo
		cl.scope.OpenBlock()
		vm.cp.closures.Push(cl)
	}
	return vm.cp.closures.Last(0)
}

func (vm *Instance) canBreak() bool {
	if vm.cp.closures.Len() == 0 {
		return false
	}
	cl := vm.cp.closures.Last(0)
	return cl.loops > 0 || cl.switches > 0
}

func (vm *Instance) canContinue() bool {
	return vm.cp.closures.Len() > 0 && vm.cp.closures.Last(0).loops > 0
}

// thisFieldSlot reports whether lhs refers to the current method's own `this`
// receiver (slot 0, uncaptured, unboxed, directly inside a class method) and
// field is one of that class's declared fields. When true, the caller can
// access the field directly by slot index on the receiver.
func (vm *Instance) thisFieldSlot(lhs local, field fields.ID) (*userClass, int, bool) {
	if lhs.index != 0 || lhs.isCaptured || lhs.isBoxed || vm.cp.closures.Len() == 0 {
		return nil, 0, false
	}
	uclass := vm.cp.closures.Last(0).uclass
	if uclass == nil {
		return nil, 0, false
	}
	slot, ok := uclass.FieldOffsets[field]
	return uclass, slot, ok
}

// isDirectSelfCall reports whether fn is a non-variadic recursive call to the
// function currently being compiled, which lets the call frame skip saving and
// restoring fbr.active.
func (vm *Instance) isDirectSelfCall(fn *userFn) bool {
	return !fn.isVariadic && vm.cp.closures.Len() > 0 && fn.funcInfoStatic == vm.cp.closures.Last(0).info
}

// findCapturedNames returns the set of identifier names referenced inside any
// nested function or class method within body. Returns nil when body contains
// no nested functions or classes, allocating nothing in the common case.
func findCapturedNames(body ast.Node) map[string]bool {
	var captured map[string]bool
	collectCapturedNames(body, &captured)
	return captured
}

func collectCapturedNames(body ast.Node, captured *map[string]bool) {
	mark := func(name string) {
		if *captured == nil {
			*captured = map[string]bool{}
		}
		(*captured)[name] = true
	}
	// scanNested marks the names a nested function or class body refers to, except
	// those its own parameters (or those of a function between) shadow: a parameter
	// is visible in the whole body, so such a name can never mean an outer binding.
	// Anything else is marked even if a nested local might shadow it, which only
	// boxes more than needed.
	var scanNested func(n ast.Node, shadowed []string)
	scanNested = func(n ast.Node, shadowed []string) {
		ast.Walk(n, func(sub ast.Node) bool {
			switch s := sub.(type) {
			case ast.Ident:
				if !slices.Contains(shadowed, s.Name) {
					mark(s.Name)
				}
				if s.Name == "super" {
					mark("this")
				}
			case ast.Fn:
				scanNested(s.Action, append(slices.Clip(shadowed), s.Args...))
				return false
			case ast.ClassDefinition:
				if s.Parent != "" {
					mark(s.Parent)
					mark(s.Name)
				}
			}
			return true
		})
	}
	ast.Walk(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case ast.Fn:
			scanNested(n.Action, n.Args)
			return false
		case ast.ClassDefinition:
			if n.Parent != "" {
				mark(n.Parent)
				mark(n.Name)
			}
			scanNested(n.Action, nil)
			return false
		}
		return true
	})
}

// unit is one source file of a package being compiled
type unit struct {
	src  *diag.Source
	node ast.Package
}

// sourced is a top-level statement and the file it comes from
type sourced struct {
	src  *diag.Source
	node ast.Node
}

// runPackage compiles a package from all of its files at once and runs its
// initializers. Declarations are hoisted across the files, so any file can use
// what another declares, and imports belong to the whole package.
func (vm *Instance) runPackage(moduleID string, units []unit) (res Value, exc exception) {
	prevPkg, prevSrc := vm.cp.pkg, vm.cp.src
	prevClosures := vm.cp.closures.Len()

	defer func() {
		vm.cp.pkg, vm.cp.src = prevPkg, prevSrc
		for vm.cp.closures.Len() > prevClosures {
			vm.cp.closures.Pop()
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*CompileError); ok {
				panic(r) // EvalNode reports it as what it is
			} else if e, ok := r.(exception); ok {
				exc = e
			} else if _, ok := r.(interface{ Diagnostic() *diag.Report }); ok {
				panic(r) // an imported package's own report, such as a syntax error
			} else if e, ok := r.(error); ok {
				exc = customError("%v", e.Error())
			} else {
				exc = customError("%v", r)
			}
		}
	}()

	name := units[0].node.Name
	for _, u := range units[1:] {
		if u.node.Name != name {
			vm.cp.src = u.src
			panic(vm.compileErrorAt(u.node, "this file declares package '%s', but %s declares '%s'", u.node.Name, units[0].src.Name, name).
				withHint("every file of a package declares the same name"))
		}
	}

	vm.cp.src = units[0].src
	id := moduleID
	if id == "" {
		id = name
	}
	if existing, exists := vm.rt.packages[id]; exists {
		ce := vm.compileErrorAt(units[0].node, "package '%s' is already loaded", name)
		if existing.origin != "" {
			ce.Message = fmt.Sprintf("package '%s' is already loaded from %s", name, existing.origin)
		}
		panic(ce.withHint("give each package its own name, or use a new engine to load this one again"))
	}

	this := NewPackage(name)
	this.id = id
	this.origin = units[0].src.Name
	this.compiled = true
	vm.cp.pkg = this
	vm.rt.packages[id] = this
	prevByName, hadByName := vm.rt.byName[name]
	if !hadByName || name == "main" {
		vm.rt.byName[name] = this
	}
	loaded := false
	defer func() {
		if !loaded {
			if vm.rt.packages[id] == this {
				delete(vm.rt.packages, id)
			}
			if vm.rt.byName[name] == this {
				if hadByName {
					vm.rt.byName[name] = prevByName
				} else {
					delete(vm.rt.byName, name)
				}
			}
		}
	}()

	// imports belong to the whole package; each (path, alias) is resolved once,
	// and two distinct imports cannot bind the same local name in one package
	seen := map[string]bool{}
	bound := map[string]string{} // local bound name -> import path
	for _, u := range units {
		vm.cp.src = u.src
		for _, spec := range u.node.Imports {
			dedupKey := spec.Path + "\x00" + spec.Alias
			if seen[dedupKey] {
				continue
			}
			seen[dedupKey] = true
			vm.bindImport(this, u, spec, bound)
		}
	}

	/*
		------ Loading a package ------

		1. Functions and classes are hoisted: their names exist before anything runs.
		2. Every top-level binding is declared. One with a constant initializer
		   gets its value now, so uses of it fold while compiling.
		3. Function bodies and class methods are compiled.
		4. The remaining initializers and any other top-level statements run in
		   source order, file after file, so they can call functions and build
		   any value.

		Each step covers every file of the package before the next one starts.
	*/
	// the statements of every file, each with the file it comes from
	var code []sourced
	for _, u := range units {
		for _, n := range u.node.Code {
			code = append(code, sourced{u.src, n})
		}
	}

	// 1. allocate functions & classes
	for _, it := range code {
		vm.cp.src = it.src
		node := it.node
		if fn, isFn := node.(ast.Fn); isFn {
			if fn.Name == "" {
				panic(vm.compileErrorAt(fn, "cannot declare an fn statement with no name"))
			}
			index := fields.Get(fn.Name)
			if _, exists := this.globals[index]; exists {
				panic(vm.compileErrorAt(fn, "'%s' is declared twice in this package", fn.Name))
			}

			// create a stub for now
			info := vm.newFuncInfo(fn.Name, fn.Args, fn.IsVariadic, false)
			info.def = vm.at(fn)
			boxed := boxUserFn(userFn{funcInfoStatic: info})
			this.globals[index] = pkgGlobal{Value: &boxed, IsPublic: true, IsStatic: true, IsConstant: true}
		}

		if cls, isClass := node.(ast.ClassDefinition); isClass {
			if cls.Name == "" {
				panic(vm.compileErrorAt(cls, "cannot declare a class with no name"))
			}
			index := fields.Get(cls.Name)
			if _, exists := this.globals[index]; exists {
				panic(vm.compileErrorAt(cls, "'%s' is declared twice in this package", cls.Name))
			}

			// create a stub for now
			v := boxUserClass(newUserClass(cls.Name, vm.at(cls), cls.Args))
			this.globals[index] = pkgGlobal{Value: &v, IsPublic: true, IsStatic: true, IsConstant: true}
		}
	}

	// 2. declare variables. A constant with a constant initializer gets its
	// value now, whatever the order of declarations, so uses of it fold; every
	// other binding waits for step 4.
	deferred := map[string]bool{}
	for _, it := range code {
		vm.cp.src = it.src
		node := it.node
		if iDec, isIdentDec := node.(ast.Decl); isIdentDec {
			index := fields.Get(iDec.Name)
			if _, exists := this.globals[index]; exists {
				panic(vm.compileErrorAt(iDec, "'%s' is declared twice in this package", iDec.Name))
			}
			this.globals[index] = pkgGlobal{Value: new(Value), IsPublic: true, IsStatic: iDec.IsStatic}
			deferred[iDec.Name] = true
		}
	}
	// constants are resolved by folding even with the fast paths off: which
	// bindings can be used before their declaration is part of the language
	func() {
		defer func(inline bool) { vm.cp.inline = inline }(vm.cp.inline)
		vm.cp.inline = true
		for progress := true; progress; {
			progress = false
			for _, it := range code {
				vm.cp.src = it.src
				node := it.node
				iDec, isIdentDec := node.(ast.Decl)
				if !isIdentDec || !iDec.IsStatic || !deferred[iDec.Name] {
					continue
				}
				value, ok := vm.evaluate(iDec.Value).(Value)
				if !ok {
					continue
				}
				index := fields.Get(iDec.Name)
				global := this.globals[index]
				*global.Value = value
				global.IsConstant = true
				this.globals[index] = global
				delete(deferred, iDec.Name)
				progress = true
			}
		}
	}()

	// 3. initialize functions
	for _, it := range code {
		vm.cp.src = it.src
		node := it.node
		if fn, isFn := node.(ast.Fn); isFn {
			global := this.globals[fields.Get(fn.Name)]
			ufn := (*userFn)(global.pointer)
			vm.compileFunction(ufn.funcInfoStatic, fn.Args, fn.Action, nil)
		}
	}

	// 3b. link class inheritance first, then compile class bodies ancestor-first
	// so subclasses inherit parent body fields regardless of declaration order.
	type classWork struct {
		src *diag.Source
		cls ast.ClassDefinition
		uc  *userClass
	}
	classByPtr := map[*userClass]classWork{}
	var classOrder []*userClass
	for _, it := range code {
		vm.cp.src = it.src
		cls, isClass := it.node.(ast.ClassDefinition)
		if !isClass {
			continue
		}
		global := this.globals[fields.Get(cls.Name)]
		uclass := (*userClass)(global.pointer)
		if cls.Parent != "" {
			parentGlobal, exists := this.globals[fields.Get(cls.Parent)]
			if !exists {
				panic(vm.compileErrorAt(cls, "cannot find the parent class '%v'", cls.Parent))
			}
			parentClass, ok := parentGlobal.Value.asUserClass()
			if !ok {
				panic(vm.compileErrorAt(cls, "'%v' is %s, not a class", cls.Parent, article(*parentGlobal.Value)))
			}
			for p := parentClass; p != nil; p = p.Parent {
				if p == uclass {
					panic(vm.compileErrorAt(cls, "'%v' and '%v' inherit from each other", cls.Name, cls.Parent))
				}
			}
			uclass.Parent = parentClass
		}
		classByPtr[uclass] = classWork{src: it.src, cls: cls, uc: uclass}
		classOrder = append(classOrder, uclass)
	}
	compiledClasses := map[*userClass]bool{}
	var ensureClassCompiled func(uc *userClass)
	ensureClassCompiled = func(uc *userClass) {
		if uc == nil || compiledClasses[uc] {
			return
		}
		work, inPkg := classByPtr[uc]
		if !inPkg {
			return
		}
		if uc.Parent != nil {
			ensureClassCompiled(uc.Parent)
		}
		compiledClasses[uc] = true
		vm.cp.src = work.src
		vm.compileClassBody(uc, work.cls)
	}
	for _, uc := range classOrder {
		ensureClassCompiled(uc)
	}

	// 4. run the remaining initializers and statements in source order
	for _, u := range units {
		vm.cp.src = u.src
		init := vm.compileInit(this, u.node.Code, deferred)
		if init == nil {
			continue
		}
		if vm.cp.debugInfo {
			init.funcInfoStatic.def = site{src: u.src}
			this.inits = append(this.inits, init.funcInfoStatic)
		}
		fbr := vm.borrowFiber(vm.rt.forLoading())
		fbr.running = true // the runner is held by EvalNode
		_, exc := init.call(fbr, nil)
		fbr.running = false
		vm.releaseFiber(fbr)
		if exc != nil {
			return Value{}, exc
		}
	}
	loaded = true
	vm.rt.packageOrder = append(vm.rt.packageOrder, id)
	return Value{}, nil
}

// bindImport resolves one import of a package and binds it under its alias
// (when given) or the imported package's declared name.
func (vm *Instance) bindImport(this *Package, u unit, spec ast.ImportSpec, bound map[string]string) {
	path, pos := spec.Path, spec.Pos
	// with a resolver every import goes through it, since only the resolver
	// knows which file a path means for this importer; without one, imports
	// name packages that are already loaded
	var pkg *Package
	if vm.cp.resolver == nil {
		pkg = vm.Package(path)
		if pkg == nil {
			panic(vm.compileErrorAtPos(pos, "cannot import '%s': no package resolver is configured", path))
		}
	} else {
		vm.rt.release()
		p, err := vm.cp.resolver(path)
		vm.rt.acquire()
		if err != nil {
			vm.importFailed(u, path, pos, err)
		}
		if p == nil {
			panic(vm.compileErrorAtPos(pos, "cannot import '%s': no such package", path))
		}
		pkg = p
		if pkg.Name() == "" {
			base := filepath.Base(path)
			pkg.setName(strings.TrimSuffix(base, filepath.Ext(base)))
		}
		pkgID := pkg.ID()
		if existing, ok := vm.rt.packages[pkgID]; !ok {
			vm.rt.packages[pkgID] = pkg
			if _, exists := vm.rt.byName[pkg.Name()]; !exists {
				vm.rt.byName[pkg.Name()] = pkg
			}
		} else if existing != pkg {
			ce := vm.compileErrorAtPos(pos, "cannot import '%s': it declares package '%s', which is already loaded", path, pkg.Name())
			if existing.origin != "" {
				ce.Message += " from " + existing.origin
			}
			panic(ce.withHint("give each package its own name"))
		}
	}

	bindName := spec.Alias
	if bindName == "" {
		bindName = pkg.Name()
	}
	if prevPath, clash := bound[bindName]; clash && prevPath != path {
		panic(vm.compileErrorAtPos(pos, "import '%s' binds as '%s', which is already bound by '%s'", path, bindName, prevPath).
			withHint(fmt.Sprintf("use 'as <alias>' to give one a different name, e.g. imports(\"%s\" as %s2)", path, bindName)))
	}
	bound[bindName] = path

	v := pkg.Box()
	this.globals[fields.Get(bindName)] = pkgGlobal{Value: &v, IsPublic: false, IsStatic: true, IsConstant: true}
	if spec.Alias != "" {
		this.imports = append(this.imports, fmt.Sprintf("%s as %s", path, spec.Alias))
	} else {
		this.imports = append(this.imports, bindName)
	}

	// remember an import bound under another name than its path or declared
	// name suggests, so a script that uses the wrong name gets a helpful hint
	recordAliasHint := func(unaliased string) {
		if unaliased == "" || unaliased == bindName {
			return
		}
		if this.importedAs == nil {
			this.importedAs = map[string][2]string{}
		}
		if _, exists := this.importedAs[unaliased]; !exists {
			this.importedAs[unaliased] = [2]string{path, bindName}
		}
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(filepath.Base(path)))
	recordAliasHint(base)
	if spec.Alias != "" {
		recordAliasHint(pkg.Name())
	}
}

// importFailed stops compiling because an import could not be loaded. An error
// from inside the imported package keeps its own report, which gains a note
// pointing at the import; any other error is reported at the import.
func (vm *Instance) importFailed(u unit, path string, pos diag.Pos, err error) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		panic(err)
	}
	var reporter interface{ Diagnostic() *diag.Report }
	if errors.As(err, &reporter) {
		r := reporter.Diagnostic()
		if r.Source != nil {
			r.Notes = append(r.Notes, diag.Note{Source: u.src, Pos: pos, Text: fmt.Sprintf("'%s' is imported here", path)})
			// the frame of the importer's loading adds nothing to the trace
			if n := len(r.Trace); n > 0 && r.Trace[n-1].Function == "" {
				r.Trace = r.Trace[:n-1]
			}
			panic(err)
		}
	}
	panic(vm.compileErrorAtPos(pos, "cannot import '%s': %v", path, err))
}

// compileInit compiles the top-level statements of a package that have to run:
// bindings whose initializer is not a constant, and plain statements. Functions
// and classes were hoisted already. It returns nil when there is nothing to run.
func (vm *Instance) compileInit(pkg *Package, code []ast.Node, deferred map[string]bool) *userFn {
	var captured map[string]bool
	for _, stmt := range code {
		switch stmt := stmt.(type) {
		case ast.Fn, ast.ClassDefinition:
			continue
		case ast.Decl:
			if deferred[stmt.Name] {
				collectCapturedNames(stmt.Value, &captured)
			}
		default:
			collectCapturedNames(stmt, &captured)
		}
	}

	info := vm.newFuncInfo("init", nil, false, false)
	cl := &closure{freeVars: ds.Set[int]{}, info: info, capturedNames: captured}
	cl.scope.TrackSlots = vm.cp.debugInfo
	vm.cp.closures.Push(cl)
	cl.scope.OpenBlock()

	var steps []instruction
	for _, stmt := range code {
		switch stmt := stmt.(type) {
		case ast.Fn, ast.ClassDefinition:
			continue
		case ast.Decl:
			if !deferred[stmt.Name] {
				continue
			}
			target := pkg.globals[fields.Get(stmt.Name)].Value
			value := vm.compile(stmt.Value)
			steps = append(steps, func(fbr *fiber) (Value, exception) {
				v, exc := value(fbr)
				if exc != nil {
					return v, exc
				}
				*target = v
				return Value{}, nil
			})
		default:
			steps = append(steps, vm.compile(stmt))
		}
	}

	closure := vm.cp.closures.Pop()
	if len(steps) == 0 {
		return nil
	}
	info.code = func(fbr *fiber) (Value, exception) {
		for _, step := range steps {
			if v, exc := step(fbr); exc != nil {
				return v, exc
			}
		}
		return Value{}, nil
	}
	vm.closeFunction(info, closure)
	return &userFn{funcInfoStatic: info}
}

// inheritParentFields copies any body fields declared on parent (and its
// ancestors) that are not already declared on uc, along with their constant
// defaults and dynamic field initializers.
func inheritParentFields(uc, parent *userClass) {
	if parent == nil {
		return
	}
	if len(uc.Defaults) < len(uc.Fields)-uc.ParamCount {
		grown := make([]Value, len(uc.Fields)-uc.ParamCount)
		copy(grown, uc.Defaults)
		uc.Defaults = grown
	}
	for i := parent.ParamCount; i < len(parent.Fields); i++ {
		fid := parent.Fields[i]
		if _, exists := uc.FieldOffsets[fid]; exists {
			continue
		}
		slot := len(uc.Fields)
		uc.Fields = append(uc.Fields, fid)
		uc.StaticFields = append(uc.StaticFields, parent.StaticFields[i])
		uc.FieldOffsets[fid] = slot
		var def Value
		if di := i - parent.ParamCount; di < len(parent.Defaults) {
			def = parent.Defaults[di]
		}
		uc.Defaults = append(uc.Defaults, def)
	}
	if len(parent.FieldInits) > 0 {
		uc.FieldInits = append(uc.FieldInits, parent.FieldInits...)
	}
}

// compileClassBody registers body field declarations, inherits parent body
// fields, compiles any dynamic body field initializers, and compiles methods.
func (vm *Instance) compileClassBody(uclass *userClass, cls ast.ClassDefinition) (hasCaptures bool) {
	var bodyDecls []ast.Decl
	var methodNodes []ast.Fn

	if block, ok := cls.Action.(ast.Block); ok {
		for _, stmt := range block.Code {
			switch s := stmt.(type) {
			case ast.Decl:
				fid := fields.Get(s.Name)
				if _, exists := uclass.FieldOffsets[fid]; exists {
					panic(vm.compileErrorAt(s, "field '%s' is declared twice on %s", s.Name, cls.Name))
				}
				slot := len(uclass.Fields)
				uclass.Fields = append(uclass.Fields, fid)
				uclass.StaticFields = append(uclass.StaticFields, s.IsStatic)
				uclass.FieldOffsets[fid] = slot
				bodyDecls = append(bodyDecls, s)
			case ast.Fn:
				if s.Name == "" {
					panic(vm.compileErrorAt(s, "a class method needs a name"))
				}
				methodNodes = append(methodNodes, s)
			default:
				panic(vm.compileErrorAt(stmt, "only methods ('fn') and field declarations are allowed inside a class body"))
			}
		}
	}

	// Ensure Defaults has one entry per own body field before inheriting parent fields.
	uclass.Defaults = make([]Value, len(uclass.Fields)-uclass.ParamCount)
	inheritParentFields(uclass, uclass.Parent)

	// Separate constant body field defaults from dynamic per-instance initializers.
	// Evaluate inside the <fields> closure so `this` resolves cleanly when checking
	// whether a body field initializer is a compile-time constant.
	if len(bodyDecls) > 0 {
		var captured map[string]bool
		for _, decl := range bodyDecls {
			collectCapturedNames(decl.Value, &captured)
		}
		info := vm.newFuncInfo("<fields>", []string{"this"}, false, true)
		info.def = uclass.def
		cl := &closure{freeVars: ds.Set[int]{}, info: info, uclass: uclass, capturedNames: captured}
		cl.scope.TrackSlots = vm.cp.debugInfo
		vm.cp.closures.Push(cl)
		cl.scope.OpenBlock()
		cl.scope.Declare("this", true)

		parentUnknown := uclass.declClosure != nil
		parentHasDyn := uclass.Parent != nil && len(uclass.Parent.FieldInits) > 0

		thisLocal := local{index: 0, isBoxed: cl.mayEscape("this"), isStatic: true}
		var steps []instruction
		for _, decl := range bodyDecls {
			fid := fields.Get(decl.Name)
			slot := uclass.FieldOffsets[fid]
			_, parentHadField := func() (int, bool) {
				if uclass.Parent == nil {
					return 0, false
				}
				return uclass.Parent.slot(fid)
			}()
			if !parentUnknown && (!parentHasDyn || !parentHadField) {
				if val, ok := vm.evaluate(decl.Value).(Value); ok && val.isImmutableConstant() {
					uclass.Defaults[slot-uclass.ParamCount] = val
					continue
				}
			}
			valInst := vm.compile(decl.Value)
			ownSlot := slot
			steps = append(steps, func(fbr *fiber) (Value, exception) {
				v, exc := valInst(fbr)
				if exc != nil {
					return v, exc
				}
				ci, _ := fbr.get(thisLocal).asClassInstance()
				if ci.InstanceOf == uclass {
					ci.Fields[ownSlot] = v
					return Value{}, nil
				}
				if subSlot, ok := ci.InstanceOf.slot(fid); ok && subSlot >= ci.InstanceOf.ParamCount {
					ci.Fields[subSlot] = v
				}
				return Value{}, nil
			})
		}

		closed := vm.cp.closures.Pop()
		if len(steps) > 0 {
			info.code = func(fbr *fiber) (Value, exception) {
				for _, step := range steps {
					if v, exc := step(fbr); exc != nil {
						return v, exc
					}
				}
				return Value{}, nil
			}
			vm.closeFunction(info, closed)
			fieldInit := &userFn{funcInfoStatic: info}
			if len(fieldInit.captures) > 0 {
				hasCaptures = true
			}
			uclass.FieldInit = fieldInit
			uclass.FieldInits = append(uclass.FieldInits, fieldInit)
		}
	}

	for _, mFn := range methodNodes {
		fid := fields.Get(mFn.Name)
		if _, isField := uclass.FieldOffsets[fid]; isField {
			panic(vm.compileErrorAt(mFn, "cannot declare method '%s' on %s: a field with that name already exists", mFn.Name, cls.Name))
		}
		if _, exists := uclass.Methods[fid]; exists {
			panic(vm.compileErrorAt(mFn, "method '%s' is declared twice on %s", mFn.Name, cls.Name))
		}
		methodFn := vm.compileClassMethod(uclass, mFn)
		if len(methodFn.captures) > 0 {
			hasCaptures = true
		}
		uclass.Methods[fid] = methodFn
	}
	return hasCaptures
}

func (vm *Instance) emitClass(cls ast.ClassDefinition) instruction {
	if cls.Name == "" {
		panic(vm.compileErrorAt(cls, "cannot declare a class with no name"))
	}

	enclosing := vm.currentClosure()
	index, ok := enclosing.scope.Declare(cls.Name, true)
	if !ok {
		panic(vm.compileErrorAt(cls, "double declaration of %s", cls.Name))
	}

	uclass := newUserClass(cls.Name, vm.at(cls), cls.Args)
	if vm.cp.debugInfo {
		enclosing.info.ensureDebug().classes = append(enclosing.info.ensureDebug().classes, uclass)
	}

	var parentInst instruction
	if cls.Parent != "" {
		if cls.Parent == cls.Name {
			panic(vm.compileErrorAt(cls, "'%v' and '%v' inherit from each other", cls.Name, cls.Parent))
		}
		parentBinding, err := vm.cp.reach(cls.Parent)
		if err != nil {
			panic(vm.compileErrorAt(cls, "cannot find the parent class '%v'", cls.Parent))
		}
		if pg, isGlobal := parentBinding.(pkgGlobal); isGlobal && pg.IsConstant {
			parentClass, ok := pg.Value.asUserClass()
			if !ok {
				panic(vm.compileErrorAt(cls, "'%v' is %s, not a class", cls.Parent, article(*pg.Value)))
			}
			uclass.Parent = parentClass
		} else {
			parentInst = vm.emitIdentGet(ast.Ident{Pos: cls.Pos, Name: cls.Parent})
			uclass.declClosure = enclosing
			uclass.declIndex = index
		}
	}

	hasCaptures := vm.compileClassBody(uclass, cls)

	site := vm.at(cls)
	escapes := enclosing.mayEscape(cls.Name)
	if parentInst == nil && !hasCaptures {
		boxed := boxUserClass(uclass)
		if escapes {
			return func(fbr *fiber) (Value, exception) {
				fbr.setEscapingLocal(index, boxed)
				return Value{}, nil
			}
		}
		return func(fbr *fiber) (Value, exception) {
			fbr.setLocal(index, boxed)
			return Value{}, nil
		}
	}

	bindCaptures := func(fbr *fiber, tmpl *userFn) *userFn {
		if len(tmpl.captures) == 0 {
			return tmpl
		}
		captured := make([]*Value, len(tmpl.captures))
		for i, ref := range tmpl.captures {
			if ref.isLocal {
				captured[i] = fbr.getLocalByRef(ref.index)
			} else {
				captured[i] = fbr.getCapturedByRef(ref.index)
			}
		}
		return &userFn{funcInfoStatic: tmpl.funcInfoStatic, references: captured}
	}

	return func(fbr *fiber) (Value, exception) {
		parent := uclass.Parent
		if parentInst != nil {
			pVal, exc := parentInst(fbr)
			if exc != nil {
				return pVal, exc
			}
			parentClass, ok := pVal.asUserClass()
			if !ok {
				return Value{}, errorAt(site, "'%v' is %s, not a class", cls.Parent, article(pVal))
			}
			parent = parentClass
		}

		runtimeClass := &userClass{
			Name:         uclass.Name,
			def:          uclass.def,
			Parent:       parent,
			ParamCount:   uclass.ParamCount,
			Fields:       slices.Clone(uclass.Fields),
			StaticFields: slices.Clone(uclass.StaticFields),
			Defaults:     slices.Clone(uclass.Defaults),
			FieldOffsets: make(map[fields.ID]int, len(uclass.FieldOffsets)),
			Methods:      make(map[fields.ID]*userFn, len(uclass.Methods)),
		}
		for k, v := range uclass.FieldOffsets {
			runtimeClass.FieldOffsets[k] = v
		}
		if parentInst != nil && parent != nil {
			inheritParentFields(runtimeClass, parent)
		} else if parent != nil && len(parent.FieldInits) > 0 {
			runtimeClass.FieldInits = append(runtimeClass.FieldInits, parent.FieldInits...)
		}

		if escapes {
			cell := fbr.newEscapingLocal(index)
			*cell = boxUserClass(runtimeClass)
		} else {
			fbr.setLocal(index, boxUserClass(runtimeClass))
		}

		if uclass.FieldInit != nil {
			boundInit := bindCaptures(fbr, uclass.FieldInit)
			runtimeClass.FieldInit = boundInit
			runtimeClass.FieldInits = append(runtimeClass.FieldInits, boundInit)
		}
		for id, tmpl := range uclass.Methods {
			runtimeClass.Methods[id] = bindCaptures(fbr, tmpl)
		}

		return Value{}, nil
	}
}
