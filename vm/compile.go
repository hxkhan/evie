package vm

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/ds"
	"github.com/hxkhan/evie/token"
	"github.com/hxkhan/evie/vm/fields"
)

type instruction func(fbr *Fiber) (Value, Exception)

func (vm *Instance) compile(node ast.Node) instruction {
	switch node := node.(type) {
	case ast.Package:
		panic("cannot compile package here")

	case ast.Input[bool]:
		value := BoxBool(node.Value)
		return func(fbr *Fiber) (Value, Exception) {
			return value, nil
		}

	case ast.Input[float64]:
		value := BoxNumber(node.Value)
		return func(fbr *Fiber) (Value, Exception) {
			return value, nil
		}

	case ast.Input[string]:
		value := BoxString(node.Value)
		return func(fbr *Fiber) (Value, Exception) {
			return value, nil
		}

	case ast.Input[struct{}]:
		return func(fbr *Fiber) (Value, Exception) {
			return Value{}, nil
		}

	case ast.Break:
		return func(fbr *Fiber) (Value, Exception) {
			return Value{}, signalBreak
		}

	case ast.Continue:
		return func(fbr *Fiber) (Value, Exception) {
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
	}

	panic(fmt.Errorf("implement %T", node))
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

// IsPathImport reports whether an import names a file or folder, such as
// "./util" or "../lib/text", rather than a package by name, such as "io"
func IsPathImport(path string) bool {
	return strings.HasPrefix(path, ".") || strings.ContainsAny(path, "/\\") || filepath.IsAbs(path)
}

// runPackage compiles a package from all of its files at once and runs its
// initializers. Declarations are hoisted across the files, so any file can use
// what another declares, and imports belong to the whole package.
func (vm *Instance) runPackage(units []unit) (res Value, exc Exception) {
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
			} else if e, ok := r.(Exception); ok {
				exc = e
			} else if _, ok := r.(interface{ Diagnostic() *diag.Report }); ok {
				panic(r) // an imported package's own report, such as a syntax error
			} else if e, ok := r.(error); ok {
				exc = CustomError("%v", e.Error())
			} else {
				exc = CustomError("%v", r)
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
	if existing, exists := vm.rt.packages[name]; exists {
		ce := vm.compileErrorAt(units[0].node, "package '%s' is already loaded", name)
		if existing.origin != "" {
			ce.Message = fmt.Sprintf("package '%s' is already loaded from %s", name, existing.origin)
		}
		panic(ce.withHint("give each package its own name, or use a new engine to load this one again"))
	}

	this := NewPackage(name)
	this.origin = units[0].src.Name
	vm.cp.pkg = this
	vm.rt.packages[name] = this
	loaded := false
	defer func() {
		if !loaded && vm.rt.packages[name] == this {
			delete(vm.rt.packages, name)
		}
	}()

	// imports belong to the whole package; each is resolved once
	seen := map[string]bool{}
	for _, u := range units {
		vm.cp.src = u.src
		for i, path := range u.node.Imports {
			if seen[path] {
				continue
			}
			seen[path] = true
			var pos token.Pos
			if i < len(u.node.ImportPos) {
				pos = u.node.ImportPos[i]
			}
			vm.bindImport(this, u, path, pos)
		}
	}

	/*
		------ Loading a package ------

		1. Functions and structs are hoisted: their names exist before anything runs.
		2. Every top-level binding is declared. One with a constant initializer
		   gets its value now, so uses of it fold while compiling.
		3. Function bodies and struct methods are compiled.
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

	// 1. allocate functions & structs
	for _, it := range code {
		vm.cp.src = it.src
		node := it.node
		if fn, isFn := node.(ast.Fn); isFn {
			index := fields.Get(fn.Name)
			if _, exists := this.globals[index]; exists {
				panic(vm.compileErrorAt(fn, "'%s' is declared twice in this package", fn.Name))
			}

			// create a stub for now
			info := vm.newFuncInfo(fn.Name, fn.Args, fn.IsVariadic, false)
			info.def = vm.at(fn)
			boxed := BoxUserFn(UserFn{funcInfoStatic: info})
			this.globals[index] = Global{Value: &boxed, IsPublic: true, IsStatic: true, IsConstant: true}
		}

		if str, isStruct := node.(ast.StructDefinition); isStruct {
			index := fields.Get(str.Name)
			if _, exists := this.globals[index]; exists {
				panic(vm.compileErrorAt(str, "'%s' is declared twice in this package", str.Name))
			}

			args := make([]fields.ID, len(str.Args))
			for i, a := range str.Args {
				args[i] = fields.Get(a)
			}

			offsets := make(map[fields.ID]int, len(args))
			for i, fid := range args {
				offsets[fid] = i
			}

			// create a stub for now
			v := BoxUserStruct(&UserStruct{
				Name:         str.Name,
				def:          vm.at(str),
				Fields:       args,
				FieldOffsets: offsets,
				Methods:      make(map[fields.ID]*UserFn),
			})
			this.globals[index] = Global{Value: &v, IsPublic: true, IsStatic: true, IsConstant: true}
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
			this.globals[index] = Global{Value: new(Value), IsPublic: true, IsStatic: iDec.IsStatic}
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
			ufn := (*UserFn)(global.pointer)
			vm.compileFunction(ufn.funcInfoStatic, fn.Args, fn.Action, nil)
		}
	}

	// 3b. initialize struct methods and inheritance
	for _, it := range code {
		vm.cp.src = it.src
		node := it.node
		if str, isStruct := node.(ast.StructDefinition); isStruct {
			global := this.globals[fields.Get(str.Name)]
			ustruct := (*UserStruct)(global.pointer)

			if str.Parent != "" {
				parentGlobal, exists := this.globals[fields.Get(str.Parent)]
				if !exists {
					panic(vm.compileErrorAt(str, "cannot find the parent struct '%v'", str.Parent))
				}
				parentStruct, ok := parentGlobal.Value.AsUserStruct()
				if !ok {
					panic(vm.compileErrorAt(str, "'%v' is %s, not a struct", str.Parent, article(*parentGlobal.Value)))
				}
				for p := parentStruct; p != nil; p = p.Parent {
					if p == ustruct {
						panic(vm.compileErrorAt(str, "'%v' and '%v' inherit from each other", str.Name, str.Parent))
					}
				}
				ustruct.Parent = parentStruct
			}

			if block, ok := str.Action.(ast.Block); ok {
				for _, stmt := range block.Code {
					if mFn, isFn := stmt.(ast.Fn); isFn {
						if mFn.Name == "" {
							panic(vm.compileErrorAt(mFn, "a struct method needs a name"))
						}
						methodFn := vm.compileStructMethod(ustruct, mFn)
						ustruct.Methods[fields.Get(mFn.Name)] = methodFn
					}
				}
			}
		}
	}

	// 4. run the remaining initializers and statements in source order
	for _, u := range units {
		vm.cp.src = u.src
		init := vm.compileInit(this, u.node.Code, deferred)
		if init == nil {
			continue
		}
		fbr := vm.NewFiber()
		fbr.running = true // the runner is held by EvalNode
		_, exc := init.call(fbr, nil)
		fbr.running = false
		vm.ReleaseFiber(fbr)
		if exc != nil {
			return Value{}, exc
		}
	}
	loaded = true
	return Value{}, nil
}

// bindImport resolves one import of a package and binds it under the name the
// imported package declares
func (vm *Instance) bindImport(this *Package, u unit, path string, pos token.Pos) {
	var pkg *Package
	if !IsPathImport(path) {
		pkg = vm.rt.packages[path]
	}
	if pkg == nil {
		if vm.cp.resolver == nil {
			panic(vm.compileErrorAtPos(pos, "cannot import '%s': no package resolver is configured", path))
		}
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
			pkg.SetName(strings.TrimSuffix(base, filepath.Ext(base)))
		}
		if !IsPathImport(path) {
			vm.rt.packages[path] = pkg
		}
		if existing, ok := vm.rt.packages[pkg.Name()]; !ok {
			vm.rt.packages[pkg.Name()] = pkg
		} else if existing != pkg {
			ce := vm.compileErrorAtPos(pos, "cannot import '%s': it declares package '%s', which is already loaded", path, pkg.Name())
			if existing.origin != "" {
				ce.Message += " from " + existing.origin
			}
			panic(ce.withHint("give each package its own name"))
		}
	}

	v := pkg.Box()
	bindName := pkg.Name()
	this.globals[fields.Get(bindName)] = Global{Value: &v, IsPublic: false, IsStatic: true, IsConstant: true}

	// remember an import bound under another name than its path suggests, so a
	// script that uses the path's name can be told
	base := filepath.Base(path)
	if base = strings.TrimSuffix(base, filepath.Ext(base)); base != bindName {
		if this.importedAs == nil {
			this.importedAs = map[string][2]string{}
		}
		this.importedAs[base] = [2]string{path, bindName}
	}
}

// importFailed stops compiling because an import could not be loaded. An error
// from inside the imported package keeps its own report, which gains a note
// pointing at the import; any other error is reported at the import.
func (vm *Instance) importFailed(u unit, path string, pos token.Pos, err error) {
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
// and structs were hoisted already. It returns nil when there is nothing to run.
func (vm *Instance) compileInit(pkg *Package, code []ast.Node, deferred map[string]bool) *UserFn {
	info := vm.newFuncInfo("init", nil, false, false)
	vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: info})
	vm.cp.closures.Last(0).scope.OpenBlock()

	var steps []instruction
	for _, stmt := range code {
		switch stmt := stmt.(type) {
		case ast.Fn, ast.StructDefinition:
			continue
		case ast.Decl:
			if !deferred[stmt.Name] {
				continue
			}
			target := pkg.globals[fields.Get(stmt.Name)].Value
			value := vm.compile(stmt.Value)
			steps = append(steps, func(fbr *Fiber) (Value, Exception) {
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
	info.code = func(fbr *Fiber) (Value, Exception) {
		for _, step := range steps {
			if v, exc := step(fbr); exc != nil {
				return v, exc
			}
		}
		return Value{}, nil
	}
	vm.closeFunction(info, closure)
	return &UserFn{funcInfoStatic: info}
}
