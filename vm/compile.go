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

	case ast.Synced:
		return vm.emitSynced(node)

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
	prevModes := vm.cp.modes.Len()

	defer func() {
		vm.cp.pkg = prevPkg
		for vm.cp.closures.Len() > prevClosures {
			vm.cp.closures.Pop()
		}
		for vm.cp.modes.Len() > prevModes {
			vm.cp.modes.Pop()
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

	p, exists := vm.rt.packages[node.Name]
	if exists {
		if pi, ok := p.(*packageInstance); ok {
			vm.cp.pkg = pi
		}
	} else {
		vm.cp.pkg = &packageInstance{
			name:    node.Name,
			globals: map[fields.ID]Global{},
		}
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
			vm.rt.ReleaseGIL()
			p, err := vm.cp.resolver(name)
			vm.rt.AcquireGIL()
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
		this.globals[fields.Get(bindName)] = Global{Value: &v, IsPublic: false, IsStatic: true}
		if bindName != name {
			this.globals[fields.Get(name)] = Global{Value: &v, IsPublic: false, IsStatic: true}
		}
	}

	/*
		------ Hoisting Protocol ------

		1. Allocated function symbols without initialization
		2. Allocate & initialize variables in the order that they appear
		3. Initialize functions (order doesn't matter)

		So this is not possible because the order is maintained:
			x := y + 2
			y := 10

		But this is:
			fn hop(a) {
				return a + x
			}
			x := 3

		Also, we only allow literal declarations on the top level
		This means no calling like:
			z := hop(8)
	*/

	// 1. allocate functions & structs
	for _, node := range node.Code {
		if fn, isFn := node.(ast.Fn); isFn {
			index := fields.Get(fn.Name)
			if _, exists := this.globals[index]; exists {
				panic(CompileErrorf("double declaration of %s", fn.Name))
			}

			// create a stub for now
			fn := BoxUserFn(UserFn{
				funcInfoStatic: &funcInfoStatic{
					name:       fn.Name,
					args:       fn.Args,
					isVariadic: fn.IsVariadic,
					mode:       fn.SyncMode,
					vm:         vm,
				},
			})
			this.globals[index] = Global{Value: &fn, IsPublic: true, IsStatic: true}
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
			this.globals[index] = Global{Value: &v, IsPublic: true, IsStatic: true}
		}
	}

	// 2. allocate & initialize variables
	for _, node := range node.Code {
		if iDec, isIdentDec := node.(ast.Decl); isIdentDec {
			// check if contains function calls
			if !ast.IsCallFree(iDec.Value) {
				panic(CompileErrorf("declaration of %s contains functions calls", iDec.Name))
			}

			index := fields.Get(iDec.Name)
			if _, exists := this.globals[index]; exists {
				panic(CompileErrorf("double declaration of %s", iDec.Name))
			}

			var value Value
			switch v := vm.evaluate(iDec.Value).(type) {
			case Value:
				value = v
			case Global:
				value = *v.Value
			default:
				panic(CompileErrorf("top-level declaration '%s' cannot be statically evaluated", iDec.Name))
			}
			// store the value
			this.globals[index] = Global{Value: &value, IsPublic: true, IsStatic: iDec.IsStatic}
		}
	}

	// 3. initialize functions
	for _, node := range node.Code {
		if fn, isFn := node.(ast.Fn); isFn {
			global := this.globals[fields.Get(fn.Name)]
			ufn := (*UserFn)(global.pointer)

			vm.cp.modes.Push(ufn.mode)
			vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: ufn.funcInfoStatic})
			vm.cp.closures.Last(0).scope.OpenBlock()

			// declare the fn arguments and only then compile the code
			for _, arg := range fn.Args {
				vm.cp.closures.Last(0).scope.Declare(arg, false)
			}

			ufn.code = vm.compile(fn.Action)
			closure := vm.cp.closures.Pop()
			vm.cp.modes.Pop()
			capacity := closure.scope.Capacity()
			ufn.locals = make([]bool, capacity)

			// mark escapee variables
			recyclable := 0
			for index := range capacity {
				if closure.freeVars.Has(index) {
					ufn.locals[index] = true
					vm.log.escapesf("CT: fn %v => Local(%v) escapes\n", fn.Name, index)
				} else {
					recyclable++
				}
			}
			ufn.recyclable = recyclable
		}
	}

	// 4. initialize struct methods and inheritance
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
	return Value{}, nil
}
