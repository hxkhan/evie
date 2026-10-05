package vm

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/ds"
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

func (vm *Instance) runPackage(node ast.Package) (res Value, exc Exception) {
	prevPkg := vm.cp.pkg
	prevClosures := vm.cp.closures.Len()

	defer func() {
		vm.cp.pkg = prevPkg
		for vm.cp.closures.Len() > prevClosures {
			vm.cp.closures.Pop()
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*CompileError); ok {
				exc = &Error{Message: ce.Error()}
			} else if e, ok := r.(Exception); ok {
				exc = e
			} else if e, ok := r.(error); ok {
				exc = CustomError("%v", e.Error())
			} else {
				exc = CustomError("%v", r)
			}
		}
	}()

	if p, exists := vm.rt.packages[node.Name]; exists {
		vm.cp.pkg = p
	} else {
		vm.cp.pkg = NewPackage(node.Name)
		vm.rt.packages[node.Name] = vm.cp.pkg
	}

	this := vm.cp.pkg
	// first make sure all static imports are resolved
	for _, name := range node.Imports {
		pkg := vm.rt.packages[name]
		if pkg == nil {
			if vm.cp.resolver == nil {
				return Value{}, RuntimeExceptionF("no package resolver configured to import '%v'", name)
			}
			vm.rt.release()
			p, err := vm.cp.resolver(name)
			vm.rt.acquire()
			if err != nil {
				return Value{}, RuntimeExceptionF("failed to import '%v': %v", name, err)
			}
			if p == nil {
				return Value{}, RuntimeExceptionF("package '%v' not found", name)
			}
			pkg = p
			if pkg.Name() == "" {
				pkg.SetName(name)
			}
			vm.rt.packages[name] = pkg
			if pkg.Name() != "" && pkg.Name() != name {
				vm.rt.packages[pkg.Name()] = pkg
			}
		}

		v := pkg.Box()
		bindName := pkg.Name()
		if bindName == "" {
			bindName = filepath.Base(name)
			if ext := filepath.Ext(bindName); ext != "" {
				bindName = strings.TrimSuffix(bindName, ext)
			}
		}
		this.globals[fields.Get(bindName)] = Global{Value: &v, IsPublic: false, IsStatic: true, IsConstant: true}
		if bindName != name {
			this.globals[fields.Get(name)] = Global{Value: &v, IsPublic: false, IsStatic: true, IsConstant: true}
		}
	}

	/*
		------ Loading a package ------

		1. Functions and structs are hoisted: their names exist before anything runs.
		2. Every top-level binding is declared. One with a constant initializer
		   gets its value now, so uses of it fold while compiling.
		3. Function bodies and struct methods are compiled.
		4. The remaining initializers and any other top-level statements run in
		   source order, so they can call functions and build any value.
	*/

	// 1. allocate functions & structs
	for _, node := range node.Code {
		if fn, isFn := node.(ast.Fn); isFn {
			index := fields.Get(fn.Name)
			if _, exists := this.globals[index]; exists {
				panic(CompileErrorf("double declaration of %s", fn.Name))
			}

			// create a stub for now
			fn := BoxUserFn(UserFn{funcInfoStatic: vm.newFuncInfo(fn.Name, fn.Args, fn.IsVariadic, false)})
			this.globals[index] = Global{Value: &fn, IsPublic: true, IsStatic: true, IsConstant: true}
		}

		if str, isStruct := node.(ast.StructDefinition); isStruct {
			index := fields.Get(str.Name)
			if _, exists := this.globals[index]; exists {
				panic(CompileErrorf("double declaration of %s", str.Name))
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
	for _, node := range node.Code {
		if iDec, isIdentDec := node.(ast.Decl); isIdentDec {
			index := fields.Get(iDec.Name)
			if _, exists := this.globals[index]; exists {
				panic(CompileErrorf("double declaration of %s", iDec.Name))
			}
			this.globals[index] = Global{Value: new(Value), IsPublic: true, IsStatic: iDec.IsStatic}
			deferred[iDec.Name] = true
		}
	}
	for progress := true; progress; {
		progress = false
		for _, node := range node.Code {
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

	// 3. initialize functions
	for _, node := range node.Code {
		if fn, isFn := node.(ast.Fn); isFn {
			global := this.globals[fields.Get(fn.Name)]
			ufn := (*UserFn)(global.pointer)
			vm.compileFunction(ufn.funcInfoStatic, fn.Args, fn.Action, nil)
		}
	}

	// 3b. initialize struct methods and inheritance
	for _, node := range node.Code {
		if str, isStruct := node.(ast.StructDefinition); isStruct {
			global := this.globals[fields.Get(str.Name)]
			ustruct := (*UserStruct)(global.pointer)

			if str.Parent != "" {
				parentGlobal, exists := this.globals[fields.Get(str.Parent)]
				if !exists {
					panic(CustomError("undefined parent struct '%v'", str.Parent))
				}
				parentStruct, ok := parentGlobal.Value.AsUserStruct()
				if !ok {
					panic(CustomError("'%v' is not a struct", str.Parent))
				}
				for p := parentStruct; p != nil; p = p.Parent {
					if p == ustruct {
						panic(CustomError("cyclical inheritance detected between '%v' and '%v'", str.Name, str.Parent))
					}
				}
				ustruct.Parent = parentStruct
			}

			if block, ok := str.Action.(ast.Block); ok {
				for _, stmt := range block.Code {
					if mFn, isFn := stmt.(ast.Fn); isFn {
						if mFn.Name == "" {
							panic(CustomError("struct method must have a name"))
						}
						methodFn := vm.compileStructMethod(ustruct, mFn)
						ustruct.Methods[fields.Get(mFn.Name)] = methodFn
					}
				}
			}
		}
	}

	// 4. run the remaining initializers and statements in source order
	if init := vm.compileInit(this, node.Code, deferred); init != nil {
		fbr := vm.NewFiber()
		fbr.running = true // the runner is held by EvalNode
		_, exc := init.call(fbr, nil)
		fbr.running = false
		vm.ReleaseFiber(fbr)
		if exc != nil {
			return Value{}, exc
		}
	}
	return Value{}, nil
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
