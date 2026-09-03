package vm

import (
	"fmt"
	"math"
	"sort"

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

	case ast.StringTemplate:
		args := make([]instruction, len(node.Args))
		for i, arg := range node.Args {
			args[i] = vm.compile(arg)
		}

		return func(fbr *Fiber) (Value, Exception) {
			params := make([]any, len(args))

			for i, param := range args {
				res, err := param(fbr)
				if err != nil {
					return res, err
				}

				params[i] = res
			}

			return BoxString(fmt.Sprintf(node.Format, params...)), nil
		}

	case ast.Echo:
		what := vm.compile(node.Value)

		return func(fbr *Fiber) (Value, Exception) {
			v, err := what(fbr)
			if err != nil {
				return v, err
			}

			fmt.Println(v)
			return Value{}, nil
		}

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

	case ast.Break:
		return func(fbr *Fiber) (Value, Exception) {
			return Value{}, signalBreak
		}

	case ast.Continue:
		return func(fbr *Fiber) (Value, Exception) {
			return Value{}, signalContinue
		}

	case ast.Synced:
		vm.cp.modes.Push(ast.SyncedMode)
		action := vm.compile(node.Action)
		vm.cp.modes.Pop()

		return func(fbr *Fiber) (Value, Exception) {
			// check if already synced (re-entrant pass-through)
			if fbr.synced {
				return action(fbr)
			}

			// transition unsynced -> synced
			vm.rt.AcquireGIL()
			fbr.synced = true
			res, exc := action(fbr)
			fbr.synced = false
			vm.rt.ReleaseGIL()

			return res, exc
		}

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
		lhs := vm.compile(node.Lhs)
		rhs := vm.compile(node.Rhs)

		return func(fbr *Fiber) (Value, Exception) {
			lhs, exc := lhs(fbr)
			if exc != nil {
				return lhs, exc
			}

			rhs, exc := rhs(fbr)
			if exc != nil {
				return rhs, exc
			}

			// structs
			if us, isUserStruct := rhs.AsUserStruct(); isUserStruct {
				if usi, isUserStructInstance := lhs.AsUserStructInstance(); isUserStructInstance {
					for cur := usi.InstanceOf; cur != nil; cur = cur.Parent {
						if cur == us {
							return BoxBool(true), nil
						}
					}
					return BoxBool(false), nil
				}
				return BoxBool(false), nil
			}

			// built-ins
			if rhs == BoxBuiltinType(typeString) {
				_, isString := lhs.AsString()
				return BoxBool(isString), nil
			} else if rhs == BoxBuiltinType(typeError) {
				_, isError := lhs.AsError()
				return BoxBool(isError), nil
			} else if rhs == BoxBuiltinType(typeArray) {
				_, isArray := lhs.AsArray()
				return BoxBool(isArray), nil
			} else if rhs == BoxBuiltinType(typeObject) {
				_, isObj := lhs.AsObject()
				return BoxBool(isObj), nil
			} else if rhs == BoxBuiltinType(typeBuffer) {
				_, isBuf := lhs.AsBuffer()
				return BoxBool(isBuf), nil
			} else if rhs == BoxBuiltinType(typeTask) {
				_, isTask := lhs.AsTask()
				return BoxBool(isTask), nil
			}

			return BoxBool(false), nil
		}

	case ast.ArrayLiteral:
		elements := make([]instruction, len(node.Elements))
		for i, el := range node.Elements {
			elements[i] = vm.compile(el)
		}

		return func(fbr *Fiber) (Value, Exception) {
			values := make([]Value, len(elements))
			for i, el := range elements {
				v, err := el(fbr)
				if err != nil {
					return v, err
				}
				values[i] = v
			}
			return BoxArray(NewArray(values...)), nil
		}

	case ast.Object:
		items := make(map[fields.ID]instruction, len(node.Fields))
		for _, f := range node.Fields {
			items[fields.Get(f.Key)] = vm.compile(f.Value)
		}

		return func(fbr *Fiber) (Value, Exception) {
			obj := make(map[fields.ID]Value, len(items))
			for k, v := range items {
				value, err := v(fbr)
				if err != nil {
					return value, err
				}

				obj[k] = value
			}
			return BoxObject(obj), nil
		}

	case ast.Exists:
		if fa, isFieldAccess := node.Value.(ast.FieldAccess); isFieldAccess {
			value := vm.compile(fa.Lhs)
			index := fields.Get(fa.Rhs)

			return func(fbr *Fiber) (Value, Exception) {
				lhs, err := value(fbr)
				if err != nil {
					return lhs, err
				}

				if obj, ok := lhs.AsObject(); ok {
					_, exists := obj[index]
					return BoxBool(exists), nil
				}

				if obj, ok := lhs.AsUserStructInstance(); ok {
					obj.MU.RLock()
					_, exists := obj.Fields[index]
					obj.MU.RUnlock()
					return BoxBool(exists), nil
				}

				panic("implement type")
			}
		}

		if sc, isSubscript := node.Value.(ast.Subscript); isSubscript {
			target := vm.compile(sc.Lhs)
			key := vm.compile(sc.Key)
			return func(fbr *Fiber) (Value, Exception) {
				lhs, err := target(fbr)
				if err != nil {
					return lhs, err
				}
				k, err := key(fbr)
				if err != nil {
					return k, err
				}

				if obj, ok := lhs.AsObject(); ok {
					if keyStr, ok := k.AsString(); ok {
						_, exists := obj[fields.Get(keyStr)]
						return BoxBool(exists), nil
					}
					return BoxBool(false), nil
				}
				if arr, ok := lhs.AsArray(); ok {
					if idxFloat, ok := k.AsFloat64(); ok {
						idx := int(idxFloat)
						arr.MU.RLock()
						defer arr.MU.RUnlock()
						length := len(arr.Data)
						if idx < 0 {
							idx = length + idx
						}
						return BoxBool(idx >= 0 && idx < length), nil
					}
					return BoxBool(false), nil
				}
				if si, ok := lhs.AsUserStructInstance(); ok {
					if keyStr, ok := k.AsString(); ok {
						si.MU.RLock()
						_, exists := si.Fields[fields.Get(keyStr)]
						si.MU.RUnlock()
						return BoxBool(exists), nil
					}
					return BoxBool(false), nil
				}
				return BoxBool(false), nil
			}
		}

	case ast.Subscript:
		return vm.emitSubscript(node)
	}

	panic(fmt.Errorf("implement %T", node))
}

func (vm *Instance) runPackage(node ast.Package) (Value, Exception) {
	vm.cp.pkg = vm.rt.packages[node.Name]
	if vm.cp.pkg == nil {
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
			pkg = vm.cp.resolver(name).(*packageInstance)
			pkg.name = name

			// save as loaded package
			vm.rt.packages[name] = pkg
		}

		v := pkg.Box()
		this.globals[fields.Get(name)] = Global{Value: &v, IsPublic: false, IsStatic: true}
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
				panic(fmt.Errorf("double declaration of %s", fn.Name))
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
			this.globals[index] = Global{Value: &fn, IsStatic: true}
		}

		if str, isStruct := node.(ast.StructDefinition); isStruct {
			index := fields.Get(str.Name)
			if _, exists := this.globals[index]; exists {
				panic(fmt.Errorf("double declaration of %s", str.Name))
			}

			args := make([]fields.ID, len(str.Args))
			for i, a := range str.Args {
				args[i] = fields.Get(a)
			}

			// create a stub for now
			v := BoxUserStruct(&UserStruct{
				Name:    str.Name,
				Fields:  args,
				Methods: make(map[fields.ID]*UserFn),
			})
			this.globals[index] = Global{Value: &v, IsStatic: true}
		}
	}

	// 2. allocate & initialize variables
	for _, node := range node.Code {
		if iDec, isIdentDec := node.(ast.Decl); isIdentDec {
			// check if contains function calls
			if !ast.IsCallFree(iDec.Value) {
				panic(fmt.Errorf("declaration of %s contains functions calls", iDec.Name))
			}

			index := fields.Get(iDec.Name)
			if _, exists := this.globals[index]; exists {
				panic(fmt.Errorf("double declaration of %s", iDec.Name))
			}

			var value Value
			switch v := vm.evaluate(iDec.Value).(type) {
			case Value:
				value = v
			case Global:
				value = *v.Value
			default:
				panic("this cant be!!")
			}
			// store the value
			this.globals[index] = Global{Value: &value, IsStatic: iDec.IsStatic}
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

func (vm *Instance) emitIdentDec(node ast.Decl) instruction {
	index, success := vm.cp.closures.Last(0).scope.Declare(node.Name, node.IsStatic)
	if !success {
		panic(fmt.Errorf("double declaration of %s", node.Name))
	}

	// try to evaluate
	switch value := vm.evaluate(node.Value).(type) {
	case local:
		return func(fbr *Fiber) (Value, Exception) {
			fbr.setLocal(index, *fbr.get(value))
			return Value{}, nil
		}
	case Value:
		return func(fbr *Fiber) (Value, Exception) {
			fbr.setLocal(index, value)
			return Value{}, nil
		}
	case Global:
		return func(fbr *Fiber) (Value, Exception) {
			fbr.setLocal(index, *value.Value)
			return Value{}, nil
		}
	}

	// generic
	value := vm.compile(node.Value)
	return func(fbr *Fiber) (Value, Exception) {
		v, err := value(fbr)
		if err != nil {
			return v, err
		}

		fbr.setLocal(index, v)
		return Value{}, nil
	}
}

func (vm *Instance) emitIdentGet(node ast.Ident) instruction {
	variable, err := vm.cp.reach(node.Name)
	if err != nil {
		panic(err)
	}

	switch v := variable.(type) {
	case local:
		return func(fbr *Fiber) (Value, Exception) {
			return *fbr.get(v), nil
		}
	case Global:
		return func(fbr *Fiber) (Value, Exception) {
			return *v.Value, nil
		}
	}

	panic("ayo what")
}

func (vm *Instance) emitAssign(node ast.Assign) instruction {
	// handle variables
	if iGet, isIdentGet := node.Lhs.(ast.Ident); isIdentGet {
		variable, err := vm.cp.reach(iGet.Name)
		if err != nil {
			panic(err)
		}

		// compile new value
		value := vm.compile(node.Value)

		switch v := variable.(type) {
		case local:
			if v.isStatic {
				panic(fmt.Sprintf("Assignment to constant binding '%v'.", iGet.Name))
			}

			return func(fbr *Fiber) (Value, Exception) {
				value, err := value(fbr)
				if err != nil {
					return value, err
				}

				fbr.set(v, value)
				return Value{}, nil
			}

		case Global:
			if v.IsStatic {
				panic(fmt.Sprintf("Assignment to constant binding '%v'.", iGet.Name))
			}

			return func(fbr *Fiber) (Value, Exception) {
				value, err := value(fbr)
				if err != nil {
					return value, err
				}

				*(v.Value) = value
				return Value{}, nil
			}

		}
	}

	// handle field access assignments
	if fa, isFieldAccess := node.Lhs.(ast.FieldAccess); isFieldAccess {
		// Lhs can for e.g. be an ast.Ident
		switch lhs := vm.evaluate(fa.Lhs).(type) {
		case local:
			// compile new value & return setter
			value := vm.compile(node.Value)
			index := fields.Get(fa.Rhs)
			return func(fbr *Fiber) (Value, Exception) {
				lhs := fbr.get(lhs)

				if obj, ok := lhs.AsObject(); ok {
					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					obj[index] = value
					return Value{}, nil
				}

				if obj, ok := lhs.AsUserStructInstance(); ok {
					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					obj.MU.Lock()
					obj.Fields[index] = value
					obj.MU.Unlock()
					return Value{}, nil
				}

				panic("implement type")
			}

		case Global:
			if lhs.IsStatic {
				if pkg, ok := lhs.asPackage(); ok {
					field, exists := pkg.globals[fields.Get(fa.Rhs)]
					if !exists {
						panic(TypeErrorF("Symbol '%s' not found in package '%s'.", fa.Rhs, pkg.name))
					}

					if field.IsStatic {
						panic(TypeErrorF("Assignment to constant symbol '%v' of package '%v'.", fa.Rhs, pkg.name))
					}

					// compile new value & return setter
					value := vm.compile(node.Value)
					return func(fbr *Fiber) (Value, Exception) {
						value, err := value(fbr)
						if err != nil {
							return value, err
						}

						*(field.Value) = value
						return Value{}, nil
					}
				}

				panic("implement type")
			}

			// compile new value & return setter
			value := vm.compile(node.Value)
			index := fields.Get(fa.Rhs)
			return func(fbr *Fiber) (Value, Exception) {
				if pkg, ok := lhs.asPackage(); ok {
					field, exists := pkg.globals[index]
					if !exists {
						return Value{}, TypeErrorF("Symbol '%s' not found in package '%s'.", fa.Rhs, pkg.name)
					}

					if field.IsStatic {
						return Value{}, TypeErrorF("Assignment to constant symbol '%v' of package '%v'.", fa.Rhs, pkg.name)
					}

					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					*(field.Value) = value
					return Value{}, nil
				}

				if obj, ok := lhs.AsObject(); ok {
					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					obj[index] = value
					return Value{}, nil
				}

				if obj, ok := lhs.AsUserStructInstance(); ok {
					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					obj.MU.Lock()
					obj.Fields[index] = value
					obj.MU.Unlock()
					return Value{}, nil
				}

				panic("implement type")
			}
		}
	}

	// handle subscript assignments: arr[i] = val, obj[key] = val, structInstance[key] = val
	if sub, isSubscript := node.Lhs.(ast.Subscript); isSubscript {
		targetCompiled := vm.compile(sub.Lhs)
		keyCompiled := vm.compile(sub.Key)
		valCompiled := vm.compile(node.Value)

		return func(fbr *Fiber) (Value, Exception) {
			target, err := targetCompiled(fbr)
			if err != nil {
				return target, err
			}

			keyVal, err := keyCompiled(fbr)
			if err != nil {
				return keyVal, err
			}

			newVal, err := valCompiled(fbr)
			if err != nil {
				return newVal, err
			}

			// 1. Array index assignment: arr[i] = newVal
			if arr, ok := target.AsArray(); ok {
				idxFloat, ok := keyVal.AsFloat64()
				if !ok {
					return Value{}, TypeErrorF("array index must be a number, got '%s'", keyVal.TypeOf())
				}
				idx := int(idxFloat)
				if float64(idx) != idxFloat {
					return Value{}, RuntimeExceptionF("array index must be an integer, got %v", idxFloat)
				}

				arr.MU.Lock()
				defer arr.MU.Unlock()

				length := len(arr.Data)
				if idx < 0 {
					idx = length + idx
				}

				if idx < 0 || idx >= length {
					return Value{}, RuntimeExceptionF("IndexError: index %d out of bounds (length %d)", int(idxFloat), length)
				}

				arr.Data[idx] = newVal
				return Value{}, nil
			}

			// 2. Object key assignment: obj[key] = newVal
			if obj, ok := target.AsObject(); ok {
				keyStr, ok := keyVal.AsString()
				if !ok {
					return Value{}, TypeErrorF("object key must be a string, got '%s'", keyVal.TypeOf())
				}

				obj[fields.Get(keyStr)] = newVal
				return Value{}, nil
			}

			// 3. Struct instance field assignment: structInst[key] = newVal
			if si, ok := target.AsUserStructInstance(); ok {
				keyStr, ok := keyVal.AsString()
				if !ok {
					return Value{}, TypeErrorF("struct field key must be a string, got '%s'", keyVal.TypeOf())
				}

				si.MU.Lock()
				si.Fields[fields.Get(keyStr)] = newVal
				si.MU.Unlock()
				return Value{}, nil
			}

			return Value{}, TypeErrorF("cannot assign to subscript of type '%s'", target.TypeOf())
		}
	}

	panic("ayo what")
}

func (vm *Instance) emitFn(node ast.Fn) instruction {
	mode := node.SyncMode
	// inherit from parent (if not specified)
	if mode == ast.UndefinedMode {
		if vm.cp.modes.Len() > 0 {
			mode = vm.cp.modes.Last(0)
		} else {
			mode = ast.SyncedMode
		}
	}
	vm.cp.modes.Push(mode)

	// create static info object
	info := &funcInfoStatic{
		name:       node.Name,
		args:       node.Args,
		isVariadic: node.IsVariadic,
		mode:       mode,
		vm:         vm,
	}

	vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: info})
	vm.cp.closures.Last(0).scope.OpenBlock()

	// declare the fn arguments and only then compile the code
	for _, arg := range node.Args {
		vm.cp.closures.Last(0).scope.Declare(arg, false)
	}

	info.code = vm.compile(node.Action)
	closure := vm.cp.closures.Pop()
	vm.cp.modes.Pop()
	info.captures = closure.captures
	capacity := closure.scope.Capacity()

	// mark escapee variables
	info.locals = make([]bool, capacity)
	info.recyclable = 0
	for index := range capacity {
		if closure.freeVars.Has(index) {
			info.locals[index] = true
			vm.log.escapesf("CT: fn %v => Local(%v) escapes\n", node.Name, index)
		} else {
			info.recyclable++
		}
	}

	for i, ref := range closure.captures {
		vm.log.capturef("CT: closure => Capture(%v) -> %v\n", i, ref)
	}

	// create the function & return it
	if node.UsedAsExpr {
		return func(fbr *Fiber) (Value, Exception) {
			captured := make([]*Value, len(closure.captures))
			for i, ref := range closure.captures {
				var v *Value
				if ref.isLocal {
					v = fbr.getLocalByRef(ref.index)
				} else {
					v = fbr.getCapturedByRef(ref.index)
				}
				captured[i] = v
			}

			fn := UserFn{
				funcInfoStatic: info,
				references:     captured,
			}
			return BoxUserFn(fn), nil
		}
	}

	if node.Name == "" {
		panic("cannot declare an fn statement with no name")
	}

	index, ok := vm.cp.closures.Last(0).scope.Declare(node.Name, true)
	if !ok {
		panic(fmt.Errorf("double declaration of %s", node.Name))
	}

	// create the function & declare it
	return func(fbr *Fiber) (Value, Exception) {
		captured := make([]*Value, len(closure.captures))
		for i, ref := range closure.captures {
			var v *Value
			if ref.isLocal {
				v = fbr.getLocalByRef(ref.index)
			} else {
				v = fbr.getCapturedByRef(ref.index)
			}
			captured[i] = v
		}

		fn := UserFn{
			funcInfoStatic: info,
			references:     captured,
		}
		fbr.setLocal(index, BoxUserFn(fn))
		return Value{}, nil
	}
}

func (vm *Instance) compileStructMethod(ustruct *UserStruct, node ast.Fn) *UserFn {
	mode := node.SyncMode
	vm.cp.modes.Push(mode)

	methodArgs := make([]string, 1+len(node.Args))
	methodArgs[0] = "this"
	copy(methodArgs[1:], node.Args)

	info := &funcInfoStatic{
		name:       node.Name,
		args:       methodArgs,
		isVariadic: node.IsVariadic,
		mode:       mode,
		vm:         vm,
	}

	vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: info})
	vm.cp.closures.Last(0).scope.OpenBlock()

	// declare arguments: "this" is declared first as a static local at slot 0
	vm.cp.closures.Last(0).scope.Declare("this", true)
	for _, arg := range node.Args {
		vm.cp.closures.Last(0).scope.Declare(arg, false)
	}

	info.code = vm.compile(node.Action)
	closure := vm.cp.closures.Pop()
	vm.cp.modes.Pop()
	info.captures = closure.captures
	capacity := closure.scope.Capacity()

	info.locals = make([]bool, capacity)
	info.recyclable = 0
	for index := range capacity {
		if closure.freeVars.Has(index) {
			info.locals[index] = true
			vm.log.escapesf("CT: method %v.%v => Local(%v) escapes\n", ustruct.Name, node.Name, index)
		} else {
			info.recyclable++
		}
	}

	return &UserFn{
		funcInfoStatic: info,
	}
}

func (vm *Instance) emitCall(node ast.Call) instruction {
	// compile arguments
	arguments := make([]instruction, len(node.Args))
	for i, arg := range node.Args {
		arguments[i] = vm.compile(arg)
	}

	// optimise: calling static functions/bindings
	if value, ok := vm.evaluate(node.Fn).(Value); ok {
		// try evie fn
		if fn, isUserFn := value.AsUserFn(); isUserFn {
			if fn.isVariadic {
				minArgs := len(fn.args) - 1
				if len(arguments) < minArgs {
					if fn.name != "λ" {
						panic(CustomError("function '%v' requires at least %v argument(s), %v provided", fn.name, minArgs, len(arguments)))
					}
					panic(CustomError("callable requires at least %v argument(s), %v provided", minArgs, len(arguments)))
				}
			} else {
				if len(fn.args) != len(arguments) {
					if fn.name != "λ" {
						panic(CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments)))
					}
					panic(CustomError("callable requires %v argument(s), %v provided", len(fn.args), len(arguments)))
				}
			}

			// optimise: call to ourselves (recursion)
			if vm.cp.closures.Len() > 0 && fn.funcInfoStatic == vm.cp.closures.Last(0).info {
				if !fn.isVariadic {
					return func(fbr *Fiber) (result Value, exc Exception) {
						base := len(fbr.stack)
						for _, escapes := range fn.locals {
							if !escapes {
								fbr.stack = append(fbr.stack, fbr.pop())
							} else {
								fbr.stack = append(fbr.stack, &Value{})
							}
						}

						for idx, argFn := range arguments {
							arg, exc := argFn(fbr)
							if exc != nil {
								fbr.push(fn.recyclable)
								fbr.popStack(len(fn.locals))
								return arg, exc
							}

							*(fbr.stack[base+idx]) = arg
						}

						prevBase := fbr.swapBase(base)

						if fn.mode == ast.UndefinedMode || fbr.synced {
							result, exc = fn.code(fbr)
						} else {
							vm.rt.AcquireGIL()
							fbr.synced = true
							result, exc = fn.code(fbr)
							fbr.synced = false
							vm.rt.ReleaseGIL()
						}

						fbr.push(fn.recyclable)
						fbr.popStack(len(fn.locals))
						fbr.swapBase(prevBase)

						switch exc {
						case nil:
							return Value{}, nil
						case signalReturn:
							return result, nil
						default:
							return result, exc
						}
					}
				}

				return func(fbr *Fiber) (result Value, exc Exception) {
					base := len(fbr.stack)
					for idx, escapes := range fn.locals {
						if !escapes {
							fbr.stack = append(fbr.stack, fbr.pop())
						} else {
							fbr.stack = append(fbr.stack, &Value{})
						}

						variadicIdx := len(fn.args) - 1
						if idx < variadicIdx {
							arg, exc := arguments[idx](fbr)
							if exc != nil {
								return arg, exc
							}

							*(fbr.stack[base+idx]) = arg
						} else if idx == variadicIdx {
							tailCount := len(arguments) - variadicIdx
							tail := make([]Value, tailCount)
							for t := 0; t < tailCount; t++ {
								arg, exc := arguments[variadicIdx+t](fbr)
								if exc != nil {
									return arg, exc
								}
								tail[t] = arg
							}
							*(fbr.stack[base+idx]) = BoxArray(&Array{Data: tail})
						}
					}

					prevBase := fbr.swapBase(base)

					if fn.mode == ast.UndefinedMode || fbr.synced {
						result, exc = fn.code(fbr)
					} else {
						vm.rt.AcquireGIL()
						fbr.synced = true
						result, exc = fn.code(fbr)
						fbr.synced = false
						vm.rt.ReleaseGIL()
					}

					fbr.push(fn.recyclable)
					fbr.popStack(len(fn.locals))
					fbr.swapBase(prevBase)

					switch exc {
					case nil:
						return Value{}, nil
					case signalReturn:
						return result, nil
					default:
						return result, exc
					}
				}
			}

			// optimise: call to an arbitrary static global
			if !fn.isVariadic {
				return func(fbr *Fiber) (result Value, exc Exception) {
					base := len(fbr.stack)
					for _, escapes := range fn.locals {
						if !escapes {
							fbr.stack = append(fbr.stack, fbr.pop())
						} else {
							fbr.stack = append(fbr.stack, &Value{})
						}
					}

					for idx, argFn := range arguments {
						arg, exc := argFn(fbr)
						if exc != nil {
							fbr.push(fn.recyclable)
							fbr.popStack(len(fn.locals))
							return arg, exc
						}

						*(fbr.stack[base+idx]) = arg
					}

					prevBase := fbr.swapBase(base)
					prevFn := fbr.swapActive(fn)

					if fn.mode == ast.UndefinedMode || fbr.synced {
						result, exc = fn.code(fbr)
					} else {
						vm.rt.AcquireGIL()
						fbr.synced = true
						result, exc = fn.code(fbr)
						fbr.synced = false
						vm.rt.ReleaseGIL()
					}

					fbr.push(fn.recyclable)
					fbr.popStack(len(fn.locals))
					fbr.swapBase(prevBase)
					fbr.swapActive(prevFn)

					switch exc {
					case nil:
						return Value{}, nil
					case signalReturn:
						return result, nil
					default:
						return result, exc
					}
				}
			}

			return func(fbr *Fiber) (result Value, exc Exception) {
				base := len(fbr.stack)
				for idx, escapes := range fn.locals {
					if !escapes {
						fbr.stack = append(fbr.stack, fbr.pop())
					} else {
						fbr.stack = append(fbr.stack, &Value{})
					}

					variadicIdx := len(fn.args) - 1
					if idx < variadicIdx {
						arg, exc := arguments[idx](fbr)
						if exc != nil {
							return arg, exc
						}

						*(fbr.stack[base+idx]) = arg
					} else if idx == variadicIdx {
						tailCount := len(arguments) - variadicIdx
						tail := make([]Value, tailCount)
						for t := 0; t < tailCount; t++ {
							arg, exc := arguments[variadicIdx+t](fbr)
							if exc != nil {
								return arg, exc
							}
							tail[t] = arg
						}
						*(fbr.stack[base+idx]) = BoxArray(&Array{Data: tail})
					}
				}

				prevBase := fbr.swapBase(base)
				prevFn := fbr.swapActive(fn)

				if fn.mode == ast.UndefinedMode || fbr.synced {
					result, exc = fn.code(fbr)
				} else {
					vm.rt.AcquireGIL()
					fbr.synced = true
					result, exc = fn.code(fbr)
					fbr.synced = false
					vm.rt.ReleaseGIL()
				}

				fbr.push(fn.recyclable)
				fbr.popStack(len(fn.locals))
				fbr.swapBase(prevBase)
				fbr.swapActive(prevFn)

				switch exc {
				case nil:
					return Value{}, nil
				case signalReturn:
					return result, nil
				default:
					return result, exc
				}
			}
		}

		// try go func
		if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
			return func(fbr *Fiber) (Value, Exception) {
				return fn.call(fbr, arguments)
			}
		}

		// try method
		if m, isMethod := value.asMethod(); isMethod {
			return func(fbr *Fiber) (Value, Exception) {
				return m.call(fbr, arguments)
			}
		}

		// try struct
		if us, isUserStruct := value.AsUserStruct(); isUserStruct {
			if len(us.Fields) != len(arguments) {
				panic(CustomError("struct '%v' requires %v argument(s), %v provided", us.Name, len(us.Fields), len(arguments)))
			}

			return func(fbr *Fiber) (Value, Exception) {
				usi := &UserStructInstance{InstanceOf: us}
				usi.Fields = make(map[fields.ID]Value, len(us.Fields))

				for i, f := range us.Fields {
					v, exc := arguments[i](fbr)
					if exc != nil {
						return v, exc
					}

					usi.Fields[f] = v
				}

				return BoxUserStructInstance(usi), nil
			}

		}

		// try built-in
		if bt, isBuiltinType := value.AsBuiltinType(); isBuiltinType {
			return func(fbr *Fiber) (Value, Exception) {
				return bt.Constructor.call(fbr, arguments)
			}
		}

		panic("cannot call whatever this is")
	}

	// optimise: calling methods (avoids heap allocation of Method{})
	if iFA, ok := node.Fn.(ast.FieldAccess); ok {
		if lhs, ok := vm.evaluate(iFA.Lhs).(local); ok {
			index := fields.Get(iFA.Rhs)
			return func(fbr *Fiber) (Value, Exception) {
				obj := fbr.get(lhs)
				if pkg, ok := obj.asPackage(); ok {
					value, exists := pkg.globals[index]
					if !exists {
						return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", iFA.Rhs, iFA)
					}

					// try go func
					if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
						return fn.call(fbr, arguments)
					}

					panic("fix.")
				}

				// 100% method
				value := obj.dotAccess(index)
				if value.IsNil() {
					return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", iFA.Rhs, iFA)
				}
				return BoundMethod{this: *obj, fn: value}.call(fbr, arguments)
			}
		}
	}

	// generic compilation
	value := vm.compile(node.Fn)
	return func(fbr *Fiber) (result Value, exc Exception) {
		value, exc := value(fbr)
		if exc != nil {
			return value, exc
		}

		// check if it is a user function
		if fn, isUserFn := value.AsUserFn(); isUserFn {
			if fn.isVariadic {
				minArgs := len(fn.args) - 1
				if len(arguments) < minArgs {
					if fn.name != "λ" {
						return Value{}, CustomError("function '%v' requires at least %v argument(s), %v provided", fn.name, minArgs, len(arguments))
					}
					return Value{}, CustomError("callable requires at least %v argument(s), %v provided", minArgs, len(arguments))
				}
			} else {
				if len(fn.args) != len(arguments) {
					if fn.name != "λ" {
						return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
					}
					return Value{}, CustomError("callable requires %v argument(s), %v provided", len(fn.args), len(arguments))
				}
			}

			// setup stack locals
			base := len(fbr.stack)
			for idx, escapes := range fn.locals {
				if !escapes {
					fbr.stack = append(fbr.stack, fbr.pop())
				} else {
					fbr.stack = append(fbr.stack, &Value{})
				}

				// evaluate arguments
				if !fn.isVariadic {
					if idx < len(arguments) {
						arg, exc := arguments[idx](fbr)
						if exc != nil {
							return arg, exc
						}

						*(fbr.stack[base+idx]) = arg
					}
				} else {
					variadicIdx := len(fn.args) - 1
					if idx < variadicIdx {
						arg, exc := arguments[idx](fbr)
						if exc != nil {
							return arg, exc
						}

						*(fbr.stack[base+idx]) = arg
					} else if idx == variadicIdx {
						tailCount := len(arguments) - variadicIdx
						tail := make([]Value, tailCount)
						for t := 0; t < tailCount; t++ {
							arg, exc := arguments[variadicIdx+t](fbr)
							if exc != nil {
								return arg, exc
							}
							tail[t] = arg
						}
						*(fbr.stack[base+idx]) = BoxArray(&Array{Data: tail})
					}
				}
			}

			// save current state
			prevBase := fbr.swapBase(base)
			prevFn := fbr.swapActive(fn)

			// no transition
			if fn.mode == ast.UndefinedMode || fbr.synced {
				result, exc = fn.code(fbr)
			} else {
				// transition unsynced -> synced
				vm.rt.AcquireGIL()
				fbr.synced = true
				result, exc = fn.code(fbr)
				fbr.synced = false
				vm.rt.ReleaseGIL()
			}

			// restore old state
			fbr.push(fn.recyclable)
			fbr.popStack(len(fn.locals))
			fbr.swapBase(prevBase)
			fbr.swapActive(prevFn)

			// return result but catch relevant signals
			switch exc {
			case nil:
				return Value{}, nil
			case signalReturn:
				return result, nil
			default:
				return result, exc
			}
		}

		// try go func
		if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
			return fn.call(fbr, arguments)
		}

		// try method
		if m, isMethod := value.asMethod(); isMethod {
			return m.call(fbr, arguments)
		}

		return Value{}, CustomError("cannot call a non-function '%v'", value)
	}
}

func (vm *Instance) emitGo(node ast.Go) instruction {
	if node, isCall := node.Fn.(ast.Call); isCall {
		// compile arguments
		arguments := make([]instruction, len(node.Args))
		for i, arg := range node.Args {
			arguments[i] = vm.compile(arg)
		}

		spawnUserFnTask := func(fn *UserFn, params []Value) *Task {
			task := NewTaskInstance()
			vm.rt.wg.Go(func() {
				// setup new fiber
				fbr := vm.rt.fibers.Get().(*Fiber)
				fbr.active = fn
				fbr.base = 0
				fbr.stack = fbr.stack[:0]
				fbr.synced = fn.mode == ast.SyncedMode

				// unique locals
				var result Value
				var exc Exception

				// setup stack locals
				for idx, escapes := range fn.locals {
					if !escapes {
						fbr.stack = append(fbr.stack, fbr.pop())
					} else {
						fbr.stack = append(fbr.stack, &Value{})
					}

					// assign arguments
					if !fn.isVariadic {
						if idx < len(params) {
							*(fbr.stack[idx]) = params[idx]
						}
					} else {
						variadicIdx := len(fn.args) - 1
						if idx < variadicIdx {
							*(fbr.stack[idx]) = params[idx]
						} else if idx == variadicIdx {
							tailCount := len(params) - variadicIdx
							tail := make([]Value, tailCount)
							copy(tail, params[variadicIdx:])
							*(fbr.stack[idx]) = BoxArray(&Array{Data: tail})
						}
					}
				}

				// run code
				if fn.mode == ast.UndefinedMode {
					result, exc = fn.code(fbr)
				} else {
					vm.rt.AcquireGIL()
					result, exc = fn.code(fbr)
					vm.rt.ReleaseGIL()
				}

				// cleanup fiber and release
				fbr.push(fn.recyclable)
				fbr.popStack(len(fn.locals))
				vm.rt.fibers.Put(fbr)

				// return result but catch relevant signals
				switch exc {
				case nil:
				case signalReturn:
					exc = nil
				}
				task.Finish(result, exc)
			})
			return task
		}

		spawnGoFuncTask := func(fn *GoFunc, params []Value) *Task {
			task := NewTaskInstance()
			vm.rt.wg.Go(func() {
				var result Value
				var exc Exception

				// setup new fiber
				fbr := vm.rt.fibers.Get().(*Fiber)
				fbr.base = 0
				fbr.stack = fbr.stack[:0]

				for _, p := range params {
					box := fbr.pop()
					*box = p
					fbr.stack = append(fbr.stack, box)
				}

				// run code
				if fn.Mode == ast.UndefinedMode {
					result, exc = fn.Fn(fbr)
				} else {
					vm.rt.AcquireGIL()
					result, exc = fn.Fn(fbr)
					vm.rt.ReleaseGIL()
				}

				fbr.push(len(params))
				fbr.popStack(len(params))
				vm.rt.fibers.Put(fbr)

				task.Finish(result, exc)
			})
			return task
		}

		// generic compilation
		value := vm.compile(node.Fn)
		return func(fbr *Fiber) (result Value, exc Exception) {
			value, exc := value(fbr)
			if exc != nil {
				return value, exc
			}

			// check if it is a user function
			if fn, isUserFn := value.AsUserFn(); isUserFn {
				if fn.isVariadic {
					minArgs := len(fn.args) - 1
					if len(arguments) < minArgs {
						if fn.name != "λ" {
							return Value{}, CustomError("function '%v' requires at least %v argument(s), %v provided", fn.name, minArgs, len(arguments))
						}
						return Value{}, CustomError("callable requires at least %v argument(s), %v provided", minArgs, len(arguments))
					}
				} else {
					if len(fn.args) != len(arguments) {
						if fn.name != "λ" {
							return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
						}
						return Value{}, CustomError("callable requires %v argument(s), %v provided", len(fn.args), len(arguments))
					}
				}

				// evaluate arguments
				params := make([]Value, len(arguments))
				for i, argument := range arguments {
					arg, exc := argument(fbr)
					if exc != nil {
						return arg, exc
					}
					params[i] = arg
				}

				return BoxTask(spawnUserFnTask(fn, params)), nil
			}

			// try go func
			if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
				if fn.IsVariadic {
					if len(arguments) < fn.Arguments {
						return Value{}, CustomError("callable requires at least %v argument(s), %v provided", fn.Arguments, len(arguments))
					}
				} else {
					if fn.Arguments != len(arguments) {
						return Value{}, CustomError("callable requires %v argument(s), %v provided", fn.Arguments, len(arguments))
					}
				}

				// evaluate arguments
				params := make([]Value, len(arguments))
				for i, argument := range arguments {
					arg, exc := argument(fbr)
					if exc != nil {
						return arg, exc
					}
					params[i] = arg
				}

				return BoxTask(spawnGoFuncTask(fn, params)), nil
			}

			// try method
			if m, isMethod := value.asMethod(); isMethod {
				if ufn, isUserFn := m.fn.AsUserFn(); isUserFn {
					isMethodDef := len(ufn.args) > 0 && ufn.args[0] == "this"
					if isMethodDef {
						if ufn.isVariadic {
							minArgs := len(ufn.args) - 2
							if len(arguments) < minArgs {
								return Value{}, CustomError("method '%v' requires at least %v argument(s), %v provided", ufn.name, minArgs, len(arguments))
							}
						} else {
							if len(ufn.args)-1 != len(arguments) {
								return Value{}, CustomError("method '%v' requires %v argument(s), %v provided", ufn.name, len(ufn.args)-1, len(arguments))
							}
						}

						params := make([]Value, 1+len(arguments))
						params[0] = m.this
						for i, argument := range arguments {
							arg, exc := argument(fbr)
							if exc != nil {
								return arg, exc
							}
							params[1+i] = arg
						}

						return BoxTask(spawnUserFnTask(ufn, params)), nil
					} else {
						if ufn.isVariadic {
							minArgs := len(ufn.args) - 1
							if len(arguments) < minArgs {
								if ufn.name != "λ" {
									return Value{}, CustomError("function '%v' requires at least %v argument(s), %v provided", ufn.name, minArgs, len(arguments))
								}
								return Value{}, CustomError("callable requires at least %v argument(s), %v provided", minArgs, len(arguments))
							}
						} else {
							if len(ufn.args) != len(arguments) {
								if ufn.name != "λ" {
									return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", ufn.name, len(ufn.args), len(arguments))
								}
								return Value{}, CustomError("callable requires %v argument(s), %v provided", len(ufn.args), len(arguments))
							}
						}

						params := make([]Value, len(arguments))
						for i, argument := range arguments {
							arg, exc := argument(fbr)
							if exc != nil {
								return arg, exc
							}
							params[i] = arg
						}

						return BoxTask(spawnUserFnTask(ufn, params)), nil
					}
				}

				if fn, isGoFunc := m.fn.AsGoFunc(); isGoFunc {
					if fn.IsVariadic {
						if len(arguments) < fn.Arguments-1 {
							return Value{}, CustomError("method requires at least %v argument(s), %v provided", fn.Arguments-1, len(arguments))
						}
					} else {
						if fn.Arguments-1 != len(arguments) {
							return Value{}, CustomError("method requires %v argument(s), %v provided", fn.Arguments-1, len(arguments))
						}
					}

					params := make([]Value, 1+len(arguments))
					params[0] = m.this
					for i, argument := range arguments {
						arg, exc := argument(fbr)
						if exc != nil {
							return arg, exc
						}
						params[1+i] = arg
					}

					return BoxTask(spawnGoFuncTask(fn, params)), nil
				}
			}

			return Value{}, CustomError("cannot call a non-function '%v'", value)
		}
	}
	panic("go expected call, got something else")
}

func (vm *Instance) emitReturn(node ast.Return) instruction {
	if vm.cp.inline {
		// optimise: returning constants
		if in, isInput := node.Value.(ast.Input[float64]); isInput {
			value := BoxNumber(in.Value)
			return func(fbr *Fiber) (Value, Exception) {
				return value, signalReturn
			}
		}

		// optimise: returning locals variable
		if iGet, isIdentGet := node.Value.(ast.Ident); isIdentGet {
			variable, err := vm.cp.reach(iGet.Name)
			if err != nil {
				panic(err)
			}

			if v, isLocal := variable.(local); isLocal {
				if v.isCaptured {
					return func(fbr *Fiber) (Value, Exception) {
						return fbr.getCaptured(v.index), signalReturn
					}
				}
				return func(fbr *Fiber) (Value, Exception) {
					return fbr.GetLocal(v.index), signalReturn
				}
			}
		}
	}

	what := vm.compile(node.Value)
	return func(fbr *Fiber) (Value, Exception) {
		v, err := what(fbr)
		if err != nil {
			return v, err
		}

		return v, signalReturn
	}
}

func (vm *Instance) emitThrow(node ast.Throw) instruction {
	what := vm.compile(node.Value)
	return func(fbr *Fiber) (Value, Exception) {
		v, err := what(fbr)
		if err != nil {
			return v, err
		}

		if e, ok := v.AsError(); ok {
			return Value{}, e
		}

		return Value{}, CustomError("%s", v.String())
	}
}

func (vm *Instance) emitAwait(node ast.Await) instruction {
	value := vm.compile(node.Task)

	return func(fbr *Fiber) (Value, Exception) {
		v, err := value(fbr)
		if err != nil {
			return v, err
		}

		if task, ok := v.AsTask(); ok {
			return task.Await(fbr)
		}

		// Awaiting a non-task value returns the value directly (identity await)
		return v, nil
	}
}

func extractTasks(evaluated []Value) ([]*Task, []Value) {
	if len(evaluated) == 1 {
		if arr, ok := evaluated[0].AsArray(); ok {
			arr.MU.RLock()
			defer arr.MU.RUnlock()
			tasks := make([]*Task, len(arr.Data))
			raw := make([]Value, len(arr.Data))
			for i, item := range arr.Data {
				raw[i] = item
				if t, ok := item.AsTask(); ok {
					tasks[i] = t
				}
			}
			return tasks, raw
		}
	}

	tasks := make([]*Task, len(evaluated))
	for i, item := range evaluated {
		if t, ok := item.AsTask(); ok {
			tasks[i] = t
		}
	}
	return tasks, evaluated
}

func (vm *Instance) emitAwaitAll(node ast.AwaitAll) instruction {
	values := make([]instruction, len(node.Tasks))
	for i, task := range node.Tasks {
		values[i] = vm.compile(task)
	}

	return func(fbr *Fiber) (Value, Exception) {
		evaluated := make([]Value, len(values))
		for i, value := range values {
			v, exc := value(fbr)
			if exc != nil {
				return v, exc
			}
			evaluated[i] = v
		}

		tasks, raw := extractTasks(evaluated)
		results := make([]Value, len(tasks))
		for i, t := range tasks {
			if t != nil {
				res, err := t.Await(fbr)
				if err != nil {
					return res, err
				}
				results[i] = res
			} else {
				results[i] = raw[i]
			}
		}

		return BoxArray(NewArray(results...)), nil
	}
}

func (vm *Instance) emitAwaitAny(node ast.AwaitAny) instruction {
	values := make([]instruction, len(node.Tasks))
	for i, task := range node.Tasks {
		values[i] = vm.compile(task)
	}

	return func(fbr *Fiber) (Value, Exception) {
		evaluated := make([]Value, len(values))
		for i, value := range values {
			v, exc := value(fbr)
			if exc != nil {
				return v, exc
			}
			evaluated[i] = v
		}

		tasks, raw := extractTasks(evaluated)
		if len(tasks) == 0 {
			return Value{}, CustomError("await.any called with empty list of tasks")
		}

		// Check if any item is already a resolved non-task or completed task without error
		for i, t := range tasks {
			if t == nil {
				return raw[i], nil
			}
			if t.IsDone() {
				res, err := t.Result()
				if err == nil {
					return res, nil
				}
			}
		}

		type outcome struct {
			res Value
			err Exception
		}

		outcomes := make(chan outcome, len(tasks))
		for _, t := range tasks {
			go func(task *Task) {
				<-task.Done()
				res, err := task.Result()
				outcomes <- outcome{res: res, err: err}
			}(t)
		}

		if fbr.synced && fbr.vm != nil {
			fbr.vm.rt.ReleaseGIL()
		}

		var firstSuccess *outcome
		failedCount := 0
		for failedCount < len(tasks) {
			out := <-outcomes
			if out.err == nil {
				firstSuccess = &out
				break
			}
			failedCount++
		}

		if fbr.synced && fbr.vm != nil {
			fbr.vm.rt.AcquireGIL()
		}

		if firstSuccess != nil {
			return firstSuccess.res, nil
		}

		return Value{}, CustomError("all tasks failed in await.any")
	}
}

func (vm *Instance) emitAwaitRace(node ast.AwaitRace) instruction {
	values := make([]instruction, len(node.Tasks))
	for i, task := range node.Tasks {
		values[i] = vm.compile(task)
	}

	return func(fbr *Fiber) (Value, Exception) {
		evaluated := make([]Value, len(values))
		for i, value := range values {
			v, exc := value(fbr)
			if exc != nil {
				return v, exc
			}
			evaluated[i] = v
		}

		tasks, raw := extractTasks(evaluated)
		if len(tasks) == 0 {
			return Value{}, CustomError("await.race called with empty list of tasks")
		}

		// Check if any item is already a non-task or completed task
		for i, t := range tasks {
			if t == nil {
				return raw[i], nil
			}
			if t.IsDone() {
				res, err := t.Result()
				return res, err
			}
		}

		type outcome struct {
			res Value
			err Exception
		}

		outcomes := make(chan outcome, len(tasks))
		for _, t := range tasks {
			go func(task *Task) {
				<-task.Done()
				res, err := task.Result()
				outcomes <- outcome{res: res, err: err}
			}(t)
		}

		if fbr.synced && fbr.vm != nil {
			fbr.vm.rt.ReleaseGIL()
		}

		first := <-outcomes

		if fbr.synced && fbr.vm != nil {
			fbr.vm.rt.AcquireGIL()
		}

		return first.res, first.err
	}
}

func (vm *Instance) compileCondition(node ast.Node) func(fbr *Fiber) (bool, Exception) {
	if bin, ok := node.(ast.BinOp); ok {
		switch bin.Operator {
		case ast.LtOp, ast.LtEqOp, ast.GtOp, ast.GtEqOp, ast.EqOp, ast.NotEqOp:
			// optimise: lhs local and rhs constant
			if lhsLocal, isLocal := vm.evaluate(bin.Lhs).(local); isLocal {
				if rhsVal, isVal := vm.evaluate(bin.Rhs).(Value); isVal {
					if rhsNum, isNum := rhsVal.AsFloat64(); isNum {
						switch bin.Operator {
						case ast.LtOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								if lhs.pointer == f64Type {
									return math.Float64frombits(lhs.scalar) < rhsNum, nil
								}
								res, ok := lhs.LessThan(rhsVal)
								if !ok {
									return false, operatorError("<", lhs, rhsVal)
								}
								return res.IsTruthy(), nil
							}
						case ast.LtEqOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								if lhs.pointer == f64Type {
									return math.Float64frombits(lhs.scalar) <= rhsNum, nil
								}
								res, ok := lhs.LessThanOrEqualTo(rhsVal)
								if !ok {
									return false, operatorError("<=", lhs, rhsVal)
								}
								return res.IsTruthy(), nil
							}
						case ast.GtOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								if lhs.pointer == f64Type {
									return math.Float64frombits(lhs.scalar) > rhsNum, nil
								}
								res, ok := lhs.GreaterThan(rhsVal)
								if !ok {
									return false, operatorError(">", lhs, rhsVal)
								}
								return res.IsTruthy(), nil
							}
						case ast.GtEqOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								if lhs.pointer == f64Type {
									return math.Float64frombits(lhs.scalar) >= rhsNum, nil
								}
								res, ok := lhs.GreaterThanOrEqualTo(rhsVal)
								if !ok {
									return false, operatorError(">=", lhs, rhsVal)
								}
								return res.IsTruthy(), nil
							}
						case ast.EqOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								return lhs.Equals(rhsVal), nil
							}
						case ast.NotEqOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								return !lhs.Equals(rhsVal), nil
							}
						}
					}

					switch bin.Operator {
					case ast.EqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs := *fbr.get(lhsLocal)
							return lhs.Equals(rhsVal), nil
						}
					case ast.NotEqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs := *fbr.get(lhsLocal)
							return !lhs.Equals(rhsVal), nil
						}
					}
				}
			}

			// optimise: lhs local and rhs local
			if lhsLocal, isLocalL := vm.evaluate(bin.Lhs).(local); isLocalL {
				if rhsLocal, isLocalR := vm.evaluate(bin.Rhs).(local); isLocalR {
					switch bin.Operator {
					case ast.LtOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							if lhs.pointer == f64Type && rhs.pointer == f64Type {
								return math.Float64frombits(lhs.scalar) < math.Float64frombits(rhs.scalar), nil
							}
							res, ok := lhs.LessThan(rhs)
							if !ok {
								return false, operatorError("<", lhs, rhs)
							}
							return res.IsTruthy(), nil
						}
					case ast.LtEqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							if lhs.pointer == f64Type && rhs.pointer == f64Type {
								return math.Float64frombits(lhs.scalar) <= math.Float64frombits(rhs.scalar), nil
							}
							res, ok := lhs.LessThanOrEqualTo(rhs)
							if !ok {
								return false, operatorError("<=", lhs, rhs)
							}
							return res.IsTruthy(), nil
						}
					case ast.GtOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							if lhs.pointer == f64Type && rhs.pointer == f64Type {
								return math.Float64frombits(lhs.scalar) > math.Float64frombits(rhs.scalar), nil
							}
							res, ok := lhs.GreaterThan(rhs)
							if !ok {
								return false, operatorError(">", lhs, rhs)
							}
							return res.IsTruthy(), nil
						}
					case ast.GtEqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							if lhs.pointer == f64Type && rhs.pointer == f64Type {
								return math.Float64frombits(lhs.scalar) >= math.Float64frombits(rhs.scalar), nil
							}
							res, ok := lhs.GreaterThanOrEqualTo(rhs)
							if !ok {
								return false, operatorError(">=", lhs, rhs)
							}
							return res.IsTruthy(), nil
						}
					case ast.EqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							return lhs.Equals(rhs), nil
						}
					case ast.NotEqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							return !lhs.Equals(rhs), nil
						}
					}
				}
			}
		}
	}

	condition := vm.compile(node)
	return func(fbr *Fiber) (bool, Exception) {
		v, err := condition(fbr)
		if err != nil {
			return false, err
		}
		return v.IsTruthy(), nil
	}
}

func (vm *Instance) emitConditional(node ast.Conditional) instruction {
	condition := vm.compileCondition(node.Condition)
	action := vm.compile(node.Action)

	if node.Otherwise != nil {
		otherwise := vm.compile(node.Otherwise)

		return func(fbr *Fiber) (Value, Exception) {
			ok, err := condition(fbr)
			if err != nil {
				return Value{}, err
			}

			if ok {
				return action(fbr)
			}
			return otherwise(fbr)
		}
	}

	return func(fbr *Fiber) (Value, Exception) {
		ok, err := condition(fbr)
		if err != nil {
			return Value{}, err
		}

		if ok {
			return action(fbr)
		}
		return Value{}, nil
	}
}

func (vm *Instance) emitWhile(node ast.While) instruction {
	condition := vm.compileCondition(node.Condition)
	action := vm.compile(node.Action)

	return func(fbr *Fiber) (Value, Exception) {
		for {
			// evaluate condition
			ok, err := condition(fbr)
			if err != nil {
				return Value{}, err
			}

			if !ok {
				break
			}

			// evaluate action
			v, err := action(fbr)
			if err != nil {
				if err == signalContinue {
					continue
				} else if err == signalBreak {
					break
				}
				return v, err
			}
		}
		return Value{}, nil
	}
}

func (vm *Instance) emitFor(node ast.For) instruction {
	collInst := vm.compile(node.Collection)

	if vm.cp.closures.Len() == 0 {
		vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: &funcInfoStatic{name: "main"}})
		vm.cp.closures.Last(0).scope.OpenBlock()
	}

	vm.cp.closures.Last(0).scope.OpenBlock()
	defer vm.cp.closures.Last(0).scope.CloseBlock()

	var idxIndex int = -1
	if node.IndexVar != "" {
		idx, ok := vm.cp.closures.Last(0).scope.Declare(node.IndexVar, false)
		if !ok {
			panic(fmt.Errorf("double declaration of %s in for loop", node.IndexVar))
		}
		idxIndex = idx
	}

	itemIndex, ok := vm.cp.closures.Last(0).scope.Declare(node.ItemVar, false)
	if !ok {
		panic(fmt.Errorf("double declaration of %s in for loop", node.ItemVar))
	}

	bodyInst := vm.compile(node.Action)

	return func(fbr *Fiber) (Value, Exception) {
		collVal, err := collInst(fbr)
		if err != nil {
			return collVal, err
		}

		if arr, ok := collVal.AsArray(); ok {
			for i, item := range arr.Data {
				if idxIndex >= 0 {
					fbr.setLocal(idxIndex, BoxNumber(float64(i)))
				}
				fbr.setLocal(itemIndex, item)

				v, err := bodyInst(fbr)
				if err != nil {
					if err == signalContinue {
						continue
					} else if err == signalBreak {
						break
					}
					return v, err
				}
			}
			return Value{}, nil
		}

		if obj, ok := collVal.AsObject(); ok {
			keys := make([]string, 0, len(obj))
			for k := range obj {
				keys = append(keys, fields.Lookup(k))
			}
			sort.Strings(keys)

			for _, k := range keys {
				keyVal := BoxString(k)
				if idxIndex >= 0 {
					fbr.setLocal(idxIndex, keyVal)
					fbr.setLocal(itemIndex, obj[fields.Get(k)])
				} else {
					fbr.setLocal(itemIndex, keyVal)
				}

				v, err := bodyInst(fbr)
				if err != nil {
					if err == signalContinue {
						continue
					} else if err == signalBreak {
						break
					}
					return v, err
				}
			}
			return Value{}, nil
		}

		if str, ok := collVal.AsString(); ok {
			i := 0
			for _, r := range str {
				if idxIndex >= 0 {
					fbr.setLocal(idxIndex, BoxNumber(float64(i)))
				}
				fbr.setLocal(itemIndex, BoxString(string(r)))
				i++

				v, err := bodyInst(fbr)
				if err != nil {
					if err == signalContinue {
						continue
					} else if err == signalBreak {
						break
					}
					return v, err
				}
			}
			return Value{}, nil
		}

		if buf, ok := collVal.AsBuffer(); ok {
			for i, b := range buf {
				if idxIndex >= 0 {
					fbr.setLocal(idxIndex, BoxNumber(float64(i)))
				}
				fbr.setLocal(itemIndex, BoxNumber(float64(b)))

				v, err := bodyInst(fbr)
				if err != nil {
					if err == signalContinue {
						continue
					} else if err == signalBreak {
						break
					}
					return v, err
				}
			}
			return Value{}, nil
		}

		return Value{}, TypeErrorF("cannot iterate over %s", collVal.TypeOf())
	}
}

func (vm *Instance) emitBlock(node ast.Block) instruction {
	if vm.cp.closures.Len() == 0 {
		vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: &funcInfoStatic{name: "anonymous"}})
		vm.cp.closures.Last(0).scope.OpenBlock()
	}

	vm.cp.closures.Last(0).scope.OpenBlock()
	defer vm.cp.closures.Last(0).scope.CloseBlock()

	// optimise: statement extraction from block; saves an extra dispatch
	if len(node.Code) == 1 && vm.cp.inline {
		node := node.Code[0]
		// optimise: {return x}
		if ret, isReturn := node.(ast.Return); isReturn {
			// optimise: returning constants
			if in, isInput := ret.Value.(ast.Input[float64]); isInput {
				value := BoxNumber(in.Value)
				return func(fbr *Fiber) (Value, Exception) {
					return value, signalReturn
				}
			}

			// optimise: returning locals
			if iGet, isIdentGet := ret.Value.(ast.Ident); isIdentGet {
				variable, err := vm.cp.reach(iGet.Name)
				if err != nil {
					panic(err)
				}

				if v, isLocal := variable.(local); isLocal {
					if v.isCaptured {
						return func(fbr *Fiber) (Value, Exception) {
							return fbr.getCaptured(v.index), signalReturn
						}
					}
					return func(fbr *Fiber) (Value, Exception) {
						return fbr.GetLocal(v.index), signalReturn
					}
				}
			}

			what := vm.compile(ret.Value)
			return func(fbr *Fiber) (Value, Exception) {
				v, err := what(fbr)
				if err != nil {
					return v, err
				}

				return v, signalReturn
			}
		}

		// generic
		return vm.compile(node)
	}

	block := make([]instruction, len(node.Code))
	for i, statement := range node.Code {
		block[i] = vm.compile(statement)
	}

	return func(fbr *Fiber) (Value, Exception) {
		for _, statement := range block {
			if v, err := statement(fbr); err != nil {
				return v, err
			}
		}
		return Value{}, nil
	}
}

func (vm *Instance) emitFieldAccess(node ast.FieldAccess) instruction {
	index := fields.Get(node.Rhs)

	// optimise: ident as lhs
	if lhs := vm.evaluate(node.Lhs); lhs != nil {
		switch lhs := lhs.(type) {
		case local:
			return func(fbr *Fiber) (Value, Exception) {
				v := fbr.get(lhs)
				if field, exists := v.getField(index); exists {
					return field, nil
				}
				return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", node.Rhs, node)
			}

		case Value:
			// I don't think this gets used tbh because using vm.evaluate(ast.FieldAccess) bypasses this
			if field, exists := lhs.getField(index); exists {
				return func(fbr *Fiber) (Value, Exception) {
					return field, nil
				}
			}
			panic(TypeErrorF("undefined symbol '%v' in '%v'", node.Rhs, node))

		case Global:
			// global non-static binding
			return func(fbr *Fiber) (Value, Exception) {
				if field, exists := lhs.getField(index); exists {
					return field, nil
				}
				panic(TypeErrorF("undefined symbol '%v' in '%v'", node.Rhs, node))
			}
		}
	}

	// generic compilation
	lhs := vm.compile(node.Lhs)
	return func(fbr *Fiber) (Value, Exception) {
		lhs, exc := lhs(fbr)
		if exc != nil {
			return lhs, exc
		}

		if field, exists := lhs.getField(index); exists {
			return field, nil
		}

		return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", node.Rhs, node)
	}
}

func (vm *Instance) emitSubscript(node ast.Subscript) instruction {
	lhs := vm.compile(node.Lhs)
	key := vm.compile(node.Key)

	return func(fbr *Fiber) (Value, Exception) {
		target, err := lhs(fbr)
		if err != nil {
			return target, err
		}

		indexVal, err := key(fbr)
		if err != nil {
			return indexVal, err
		}

		// 1. Array indexing: arr[0], arr[-1]
		if arr, ok := target.AsArray(); ok {
			idxFloat, ok := indexVal.AsFloat64()
			if !ok {
				return Value{}, TypeErrorF("array index must be a number, got '%s'", indexVal.TypeOf())
			}
			idx := int(idxFloat)
			if float64(idx) != idxFloat {
				return Value{}, RuntimeExceptionF("array index must be an integer, got %v", idxFloat)
			}

			arr.MU.RLock()
			defer arr.MU.RUnlock()

			length := len(arr.Data)
			if idx < 0 {
				idx = length + idx
			}

			if idx < 0 || idx >= length {
				return Value{}, RuntimeExceptionF("IndexError: index %d out of bounds (length %d)", int(idxFloat), length)
			}

			return arr.Data[idx], nil
		}

		// 2. Object key access: obj["key"]
		if obj, ok := target.AsObject(); ok {
			keyStr, ok := indexVal.AsString()
			if !ok {
				return Value{}, TypeErrorF("object key must be a string, got '%s'", indexVal.TypeOf())
			}

			val, exists := obj[fields.Get(keyStr)]
			if !exists {
				return Value{}, nil
			}
			return val, nil
		}

		// 3. Struct instance field access: structInstance["field"]
		if si, ok := target.AsUserStructInstance(); ok {
			keyStr, ok := indexVal.AsString()
			if !ok {
				return Value{}, TypeErrorF("struct field key must be a string, got '%s'", indexVal.TypeOf())
			}

			si.MU.RLock()
			val, exists := si.Fields[fields.Get(keyStr)]
			si.MU.RUnlock()
			if !exists {
				return Value{}, RuntimeExceptionF("undefined field '%s' in struct", keyStr)
			}
			return val, nil
		}

		// 4. String indexing: "hello"[1] -> "e"
		if str, ok := target.AsString(); ok {
			idxFloat, ok := indexVal.AsFloat64()
			if !ok {
				return Value{}, TypeErrorF("string index must be a number, got '%s'", indexVal.TypeOf())
			}
			idx := int(idxFloat)
			if float64(idx) != idxFloat {
				return Value{}, RuntimeExceptionF("string index must be an integer, got %v", idxFloat)
			}

			length := len(str)
			if idx < 0 {
				idx = length + idx
			}

			if idx < 0 || idx >= length {
				return Value{}, RuntimeExceptionF("IndexError: string index %d out of bounds (length %d)", int(idxFloat), length)
			}

			return BoxString(string(str[idx])), nil
		}

		// 5. Buffer indexing: buf[0] -> byte value as number
		if buf, ok := target.AsBuffer(); ok {
			idxFloat, ok := indexVal.AsFloat64()
			if !ok {
				return Value{}, TypeErrorF("buffer index must be a number, got '%s'", indexVal.TypeOf())
			}
			idx := int(idxFloat)
			if float64(idx) != idxFloat {
				return Value{}, RuntimeExceptionF("buffer index must be an integer, got %v", idxFloat)
			}

			length := len(buf)
			if idx < 0 {
				idx = length + idx
			}

			if idx < 0 || idx >= length {
				return Value{}, RuntimeExceptionF("IndexError: buffer index %d out of bounds (length %d)", int(idxFloat), length)
			}

			return BoxNumber(float64(buf[idx])), nil
		}

		return Value{}, TypeErrorF("cannot index value of type '%s'", target.TypeOf())
	}
}

func (vm *Instance) emitNeg(node ast.Neg) instruction {
	value := vm.compile(node.Value)

	return func(fbr *Fiber) (Value, Exception) {
		value, exc := value(fbr)
		if exc != nil {
			return value, exc
		}

		if float, ok := value.AsFloat64(); ok {
			return BoxNumber(-float), nil
		}
		return Value{}, RuntimeExceptionF("Cannot negate '%v'.", value)
	}
}

func (vm *Instance) emitNot(node ast.Not) instruction {
	value := vm.compile(node.Value)

	return func(fbr *Fiber) (Value, Exception) {
		value, exc := value(fbr)
		if exc != nil {
			return value, exc
		}

		return BoxBool(!value.IsTruthy()), nil
	}
}

func (vm *Instance) emitCatch(node ast.Catch) instruction {
	action := vm.compile(node.Action)

	return func(fbr *Fiber) (Value, Exception) {
		v, err := action(fbr)
		if err != nil {
			if err != signalReturn && err != signalContinue && err != signalBreak {
				// convert exception to error-as-value and return
				return BoxError(err), nil
			}

			// propagate as normal
			return v, err
		}

		return v, nil
	}
}

func (vm *Instance) emitBinOp(node ast.BinOp) instruction {
	/*
		1. Local x Local
		2. Local x Value
		3. Value x Local

		With these three, we cover all possible combinations of binary operations
	*/

	if lhs := vm.evaluate(node.Lhs); lhs != nil {
		if rhs := vm.evaluate(node.Rhs); rhs != nil {
			// optimise: lhs being a local
			if lhs, isLocal := lhs.(local); isLocal {
				// optimise: rhs being a local
				if rhs, isLocal := rhs.(local); isLocal {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.Add(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("+", lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.Sub(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("-", lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.Mul(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("*", lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.Div(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("/", lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.Mod(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("%", lhs, rhs)
						}

					case ast.EqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							return BoxBool(lhs.Equals(rhs)), nil
						}

					case ast.NotEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							return BoxBool(!lhs.Equals(rhs)), nil
						}

					case ast.LtOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.LessThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.GtOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.GreaterThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError(">", lhs, rhs)
						}

					case ast.LtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.LessThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<=", lhs, rhs)
						}

					case ast.GtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.GreaterThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError(">=", lhs, rhs)
						}

					case ast.OrOp:
						return func(fbr *Fiber) (Value, Exception) {
							if fbr.get(lhs).IsTruthy() {
								return BoxBool(true), nil
							} else if fbr.get(rhs).IsTruthy() {
								return BoxBool(true), nil
							}
							return BoxBool(false), nil
						}

					case ast.AndOp:
						return func(fbr *Fiber) (Value, Exception) {
							return BoxBool(fbr.get(lhs).IsTruthy() && fbr.get(rhs).IsTruthy()), nil
						}
					}
				}

				// optimise: rhs being a constant
				if rhs, isValue := rhs.(Value); isValue {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.Add(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("+", lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.Sub(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("-", lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.Mul(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("*", lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.Div(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("/", lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.Mod(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("%", lhs, rhs)
						}

					case ast.EqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							return BoxBool(lhs.Equals(rhs)), nil
						}

					case ast.NotEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							return BoxBool(!lhs.Equals(rhs)), nil
						}

					case ast.LtOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.LessThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.GtOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.GreaterThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.LtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.LessThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<=", lhs, rhs)
						}

					case ast.GtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.GreaterThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError(">=", lhs, rhs)
						}
					}
				}
			}

			// optimise: lhs being a constant
			if lhs, isLocal := lhs.(Value); isLocal {
				if rhs, isValue := rhs.(local); isValue {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Add(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("+", lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Sub(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("-", lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Mul(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("*", lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Div(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("/", lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Mod(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("%", lhs, rhs)
						}

					case ast.EqOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							return BoxBool(lhs.Equals(rhs)), nil
						}

					case ast.NotEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							return BoxBool(!lhs.Equals(rhs)), nil
						}

					case ast.LtOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.LessThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.GtOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.GreaterThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.LtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.LessThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<=", lhs, rhs)
						}

					case ast.GtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.GreaterThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError(">=", lhs, rhs)
						}
					}
				}
			}
		}
	}

	lhs := vm.compile(node.Lhs)
	rhs := vm.compile(node.Rhs)

	// generic compilation
	switch node.Operator {
	case ast.AddOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Add(b); ok {
				return result, nil
			}
			return Value{}, operatorError("+", a, b)
		}

	case ast.SubOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Sub(b); ok {
				return result, nil
			}
			return Value{}, operatorError("-", a, b)
		}

	case ast.MulOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Mul(b); ok {
				return result, nil
			}
			return Value{}, operatorError("*", a, b)
		}

	case ast.DivOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Div(b); ok {
				return result, nil
			}
			return Value{}, operatorError("/", a, b)
		}

	case ast.ModOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Mod(b); ok {
				return result, nil
			}
			return Value{}, operatorError("%", a, b)
		}

	case ast.EqOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			return BoxBool(a.Equals(b)), nil
		}

	case ast.NotEqOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			return BoxBool(!a.Equals(b)), nil
		}

	case ast.LtOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.LessThan(b); ok {
				return result, nil
			}
			return Value{}, operatorError("<", a, b)
		}

	case ast.GtOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.GreaterThan(b); ok {
				return result, nil
			}
			return Value{}, operatorError(">", a, b)
		}

	case ast.LtEqOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.LessThanOrEqualTo(b); ok {
				return result, nil
			}
			return Value{}, operatorError("<=", a, b)
		}

	case ast.GtEqOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.GreaterThanOrEqualTo(b); ok {
				return result, nil
			}
			return Value{}, operatorError(">=", a, b)
		}

	case ast.OrOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			if a.IsTruthy() {
				return BoxBool(true), nil
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			return BoxBool(b.IsTruthy()), nil
		}

	case ast.AndOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			if !a.IsTruthy() {
				return BoxBool(false), nil
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			return BoxBool(b.IsTruthy()), nil
		}
	}

	panic(fmt.Errorf("implement Operator(%v)", node.Operator))
}

func (vm *Instance) emitMutableBinOp(node ast.MutableBinOp) instruction {
	if lhs := vm.evaluate(node.Lhs); lhs != nil {
		// optimise: lhs being a local
		if lhs, ok := lhs.(local); ok {
			if lhs.isStatic {
				panic(fmt.Sprintf("Assignment to constant binding on line '%v'.", node.Line()))
			}

			if rhs := vm.evaluate(node.Rhs); rhs != nil {
				// optimise: rhs being a local
				if rhs, ok := rhs.(local); ok {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Add(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("+", *lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Sub(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("-", *lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Mul(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("*", *lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Div(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("/", *lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Mod(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("%", *lhs, rhs)
						}
					}
				}

				// optimise: rhs being a constant
				if rhs, ok := rhs.(Value); ok {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Add(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("+", *lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Sub(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("-", *lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Mul(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("*", *lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Div(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("/", *lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Mod(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("%", *lhs, rhs)
						}
					}
				}
			}

			// generic rhs compilation for local lhs
			rhs := vm.compile(node.Rhs)
			switch node.Operator {
			case ast.AddOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					lhs := fbr.get(lhs)
					if result, ok := lhs.Add(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("+", *lhs, rhs)
				}

			case ast.SubOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					lhs := fbr.get(lhs)
					if result, ok := lhs.Sub(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("-", *lhs, rhs)
				}

			case ast.MulOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					lhs := fbr.get(lhs)
					if result, ok := lhs.Mul(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("*", *lhs, rhs)
				}

			case ast.DivOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					lhs := fbr.get(lhs)
					if result, ok := lhs.Div(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("/", *lhs, rhs)
				}

			case ast.ModOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					lhs := fbr.get(lhs)
					if result, ok := lhs.Mod(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("%", *lhs, rhs)
				}
			}
		}

		// optimise: lhs being a global
		if lhs, ok := lhs.(Global); ok {
			if lhs.IsStatic {
				panic(fmt.Sprintf("Assignment to constant binding on line '%v'.", node.Line()))
			}

			lhs := lhs.Value
			rhs := vm.compile(node.Rhs)
			switch node.Operator {
			case ast.AddOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					if result, ok := lhs.Add(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("+", *lhs, rhs)
				}

			case ast.SubOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					if result, ok := lhs.Sub(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("-", *lhs, rhs)
				}

			case ast.MulOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					if result, ok := lhs.Mul(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("*", *lhs, rhs)
				}

			case ast.DivOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					if result, ok := lhs.Div(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("/", *lhs, rhs)
				}

			case ast.ModOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					if result, ok := lhs.Mod(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("%", *lhs, rhs)
				}
			}
		}
	}

	/* lhs := vm.compile(node.Lhs)
	rhs := vm.compile(node.Rhs)

	// generic compilation
	switch node.Operator {
	case ast.AddOp:
		return func(fbr *fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Add(b); ok {
				return result, nil
			}
			return Value{}, operatorError("+", a, b)
		}

	case ast.SubOp:
		return func(fbr *fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Sub(b); ok {
				return result, nil
			}
			return Value{}, operatorError("-", a, b)
		}

	case ast.MulOp:
		return func(fbr *fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Mul(b); ok {
				return result, nil
			}
			return Value{}, operatorError("*", a, b)
		}

	case ast.DivOp:
		return func(fbr *fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Div(b); ok {
				return result, nil
			}
			return Value{}, operatorError("/", a, b)
		}

	case ast.ModOp:
		return func(fbr *fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Mod(b); ok {
				return result, nil
			}
			return Value{}, operatorError("%", a, b)
		}
	} */

	panic(fmt.Errorf("implement Operator(%v) in %v", node.Operator, node))
}
