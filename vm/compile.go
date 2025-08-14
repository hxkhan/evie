package vm

import (
	"fmt"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/ds"
	"github.com/hxkhan/evie/vm/fields"
)

type instruction func(fbr *fiber) (Value, *Exception)

func (vm *Instance) compile(node ast.Node) instruction {
	switch node := node.(type) {
	case ast.Package:
		panic("cannot compile package here")

	case ast.Input[bool]:
		value := BoxBool(node.Value)
		return func(fbr *fiber) (Value, *Exception) {
			return value, nil
		}

	case ast.Input[float64]:
		value := BoxNumber(node.Value)
		return func(fbr *fiber) (Value, *Exception) {
			return value, nil
		}

	case ast.Input[string]:
		value := BoxString(node.Value)
		return func(fbr *fiber) (Value, *Exception) {
			return value, nil
		}

	case ast.Input[struct{}]:
		return func(fbr *fiber) (Value, *Exception) {
			return Value{}, nil
		}

	case ast.StringTemplate:
		args := make([]instruction, len(node.Args))
		for i, arg := range node.Args {
			args[i] = vm.compile(arg)
		}

		return func(fbr *fiber) (Value, *Exception) {
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

		return func(fbr *fiber) (Value, *Exception) {
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

	case ast.Break:
		return func(fbr *fiber) (Value, *Exception) {
			return Value{}, breakSignal
		}

	case ast.Continue:
		return func(fbr *fiber) (Value, *Exception) {
			return Value{}, continueSignal
		}

	case ast.Unsynced:
		action := vm.compile(node.Action)
		return func(fbr *fiber) (Value, *Exception) {
			// check if already unsynchronized
			if fbr.unsynced {
				res, err := action(fbr)
				return res, err
			}

			// change mode
			vm.rt.ReleaseGIL()
			fbr.unsynced = true
			res, err := action(fbr)
			fbr.unsynced = false
			vm.rt.AcquireGIL()

			return res, err
		}

	case ast.Synced:
		action := vm.compile(node.Action)
		return func(fbr *fiber) (Value, *Exception) {
			// check if already synced
			if !fbr.unsynced {
				res, err := action(fbr)
				return res, err
			}

			// change mode
			vm.rt.AcquireGIL()
			fbr.unsynced = false
			res, err := action(fbr)
			fbr.unsynced = true
			vm.rt.ReleaseGIL()

			return res, err
		}

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

	case ast.Await:
		return vm.emitAwait(node)

	case ast.AwaitAll:
		return vm.emitAwaitAll(node)

	case ast.Neg:
		return vm.emitNeg(node)

	case ast.BinOp:
		return vm.emitBinOp(node)

	case ast.MutableBinOp:
		return vm.emitMutableBinOp(node)
	}

	panic(fmt.Errorf("implement %T", node))
}

func (vm *Instance) runPackage(node ast.Package) (Value, *Exception) {
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

			// save as loaded package
			vm.rt.packages[name] = pkg
		}

		v := pkg.Box()
		this.globals[fields.Get(name)] = Global{Value: &v, IsPublic: false, IsStatic: true}
	}

	/*
		------ Hoisting Protocol ------

		1. Allocated all symbols without initialization.
		2. Initialize variables in the order they appear
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

	// 1. allocate (functions)
	for _, node := range node.Code {
		if fn, isFn := node.(ast.Fn); isFn {
			index := fields.Get(fn.Name)
			if _, exists := this.globals[index]; exists {
				panic(fmt.Errorf("double declaration of %s", fn.Name))
			}

			// create a stub for now
			fn := BoxUserFn(UserFn{funcInfoStatic: &funcInfoStatic{
				name: fn.Name,
				args: fn.Args,
				mode: fn.SyncMode,
				vm:   vm,
			}})
			this.globals[index] = Global{Value: &fn, IsStatic: true}
		}
	}

	// 2. allocate & initialize (ident)
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

			value := vm.compile(iDec.Value)
			v, exc := value(vm.main)
			if exc != nil {
				return v, exc
			}
			// store the value
			this.globals[index] = Global{Value: v.Allocate(), IsStatic: iDec.IsStatic}
		}
	}

	// 3. initialize (functions)
	for _, node := range node.Code {
		if fn, isFn := node.(ast.Fn); isFn {
			global := this.globals[fields.Get(fn.Name)]
			ufn := (*UserFn)(global.pointer)

			vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, this: global.Value})
			vm.cp.closures.Last(0).scope.OpenBlock()

			// declare the fn arguments and only then compile the code
			for _, arg := range fn.Args {
				vm.cp.closures.Last(0).scope.Declare(arg, false)
			}

			ufn.code = vm.compile(fn.Action)
			closure := vm.cp.closures.Pop()
			ufn.capacity = closure.scope.Capacity()

			// make list of non-escaping variables so they can be recycled after execution
			ufn.recyclable = make([]int, 0, ufn.capacity-closure.freeVars.Len())
			for index := range ufn.capacity {
				if closure.freeVars.Has(index) {
					vm.log.escapesf("CT: fn %v => Local(%v) escapes\n", fn.Name, index)
					continue
				}
				ufn.recyclable = append(ufn.recyclable, index)
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
		return func(fbr *fiber) (Value, *Exception) {
			fbr.storeLocal(index, fbr.get(value))
			return Value{}, nil
		}
	case Value:
		return func(fbr *fiber) (Value, *Exception) {
			fbr.storeLocal(index, value)
			return Value{}, nil
		}
	case Global:
		return func(fbr *fiber) (Value, *Exception) {
			fbr.storeLocal(index, *value.Value)
			return Value{}, nil
		}
	}

	// generic
	value := vm.compile(node.Value)
	return func(fbr *fiber) (Value, *Exception) {
		v, err := value(fbr)
		if err != nil {
			return v, err
		}

		fbr.storeLocal(index, v)
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
		if v.isCaptured {
			return func(fbr *fiber) (Value, *Exception) {
				return fbr.getCaptured(v.index), nil
			}
		}
		return func(fbr *fiber) (Value, *Exception) {
			return fbr.getLocal(v.index), nil
		}
	case Global:
		return func(fbr *fiber) (Value, *Exception) {
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

			if v.isCaptured {
				return func(fbr *fiber) (Value, *Exception) {
					value, err := value(fbr)
					if err != nil {
						return value, err
					}

					fbr.storeCaptured(v.index, value)
					return Value{}, nil
				}
			}
			return func(fbr *fiber) (Value, *Exception) {
				value, err := value(fbr)
				if err != nil {
					return value, err
				}

				fbr.storeLocal(v.index, value)
				return Value{}, nil
			}

		case Global:
			if v.IsStatic {
				panic(fmt.Sprintf("Assignment to constant binding '%v'.", iGet.Name))
			}

			return func(fbr *fiber) (Value, *Exception) {
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
		if iGet, isIdentGet := fa.Lhs.(ast.Ident); isIdentGet {
			variable, err := vm.cp.reach(iGet.Name)
			if err != nil {
				panic(err)
			}

			switch lhs := variable.(type) {
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
						return func(fbr *fiber) (Value, *Exception) {
							value, err := value(fbr)
							if err != nil {
								return value, err
							}

							*(field.Value) = value
							return Value{}, nil
						}
					}

					panic("not a package")
				}

				// compile new value & return setter
				value := vm.compile(node.Value)
				index := fields.Get(fa.Rhs)
				return func(fbr *fiber) (Value, *Exception) {
					if pkg, ok := lhs.asPackage(); ok {
						field, exists := pkg.globals[index]
						if !exists {
							panic(TypeErrorF("Symbol '%s' not found in package '%s'.", fa.Rhs, pkg.name))
						}

						if field.IsStatic {
							panic(TypeErrorF("Assignment to constant symbol '%v' of package '%v'.", fa.Rhs, pkg.name))
						}

						value, err := value(fbr)
						if err != nil {
							return value, err
						}

						*(field.Value) = value
						return Value{}, nil
					}
					panic("not a package")
				}
			}
		}
	}

	panic("ayo what")
}

func (vm *Instance) emitFn(node ast.Fn) instruction {
	vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}})
	vm.cp.closures.Last(0).scope.OpenBlock()

	// declare the fn arguments and only then compile the code
	for _, arg := range node.Args {
		vm.cp.closures.Last(0).scope.Declare(arg, false)
	}

	action := vm.compile(node.Action)
	closure := vm.cp.closures.Pop()
	capacity := closure.scope.Capacity()

	// make list of non-escaping variables so they can be recycled after execution
	recyclable := make([]int, 0, capacity-closure.freeVars.Len())
	for index := range capacity {
		if closure.freeVars.Has(index) {
			vm.log.escapesf("CT: closure => Local(%v) escapes\n", index)
			continue
		}
		recyclable = append(recyclable, index)
	}

	info := &funcInfoStatic{
		name:       node.Name,
		args:       node.Args,
		captures:   closure.captures,
		recyclable: recyclable,
		capacity:   capacity,
		code:       action,
		vm:         vm,
	}

	for i, ref := range closure.captures {
		vm.log.capturef("CT: closure => Capture(%v) -> %v\n", i, ref)
	}

	// create the function & return it
	if node.UsedAsExpr {
		return func(fbr *fiber) (Value, *Exception) {
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
	return func(fbr *fiber) (Value, *Exception) {
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
		fbr.storeLocal(index, BoxUserFn(fn))
		return Value{}, nil
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
			if len(fn.args) != len(arguments) {
				if fn.name != "λ" {
					panic(CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments)))
				}
				panic(CustomError("function requires %v argument(s), %v provided", len(fn.args), len(arguments)))
			}

			// optimise: call to ourselves (recursion)
			if vm.cp.closures.Len() > 0 && value == *(vm.cp.closures.Last(0).this) {
				return func(fbr *fiber) (result Value, exc *Exception) {
					// setup stack locals
					base := len(fbr.stack)
					for idx := range fn.capacity {
						fbr.stack = append(fbr.stack, fbr.newValue())

						// evaluate arguments
						if idx < len(arguments) {
							arg, exc := arguments[idx](fbr)
							if exc != nil {
								return arg, exc
							}

							*(fbr.stack[base+idx]) = arg
						}
					}

					// save currently captured values
					prevBase := fbr.swapBase(base)

					// correctly invoke ourselves (mode can still change)
					switch {
					// sync -> unsync
					case !fbr.unsynced && fn.mode == ast.UnsyncedMode:
						vm.rt.ReleaseGIL()
						fbr.unsynced = true
						result, exc = fn.code(fbr)
						fbr.unsynced = false
						vm.rt.AcquireGIL()

					// unsync -> sync
					case fbr.unsynced && fn.mode == ast.SyncedMode:
						vm.rt.AcquireGIL()
						fbr.unsynced = false
						result, exc = fn.code(fbr)
						fbr.unsynced = true
						vm.rt.ReleaseGIL()

					// others
					default:
						result, exc = fn.code(fbr)
					}

					// release non-escaping locals
					for _, idx := range fn.recyclable {
						fbr.putValue(fbr.stack[base+idx])
					}

					// restore old state
					fbr.popStack(fn.capacity)
					fbr.swapBase(prevBase)

					// return result but catch relevant signals
					switch exc {
					case returnSignal:
						return result, nil
					default:
						return result, exc
					}
				}
			}

			// optimise: call to an arbitrary static global
			return func(fbr *fiber) (result Value, exc *Exception) {
				// setup stack locals
				base := len(fbr.stack)
				for idx := range fn.capacity {
					fbr.stack = append(fbr.stack, fbr.newValue())

					// evaluate arguments
					if idx < len(arguments) {
						arg, exc := arguments[idx](fbr)
						if exc != nil {
							return arg, exc
						}

						*(fbr.stack[base+idx]) = arg
					}
				}

				// save currently captured values
				prevBase := fbr.swapBase(base)
				prevFn := fbr.swapActive(fn)

				// correctly invoke the function
				switch {
				// sync -> unsync
				case !fbr.unsynced && fn.mode == ast.UnsyncedMode:
					vm.rt.ReleaseGIL()
					fbr.unsynced = true
					result, exc = fn.code(fbr)
					fbr.unsynced = false
					vm.rt.AcquireGIL()

				// unsync -> sync
				case fbr.unsynced && fn.mode == ast.SyncedMode:
					vm.rt.AcquireGIL()
					fbr.unsynced = false
					result, exc = fn.code(fbr)
					fbr.unsynced = true
					vm.rt.ReleaseGIL()

				// others
				default:
					result, exc = fn.code(fbr)
				}

				// release non-escaping locals
				for _, idx := range fn.recyclable {
					fbr.putValue(fbr.stack[base+idx])
				}

				// restore old state
				fbr.popStack(fn.capacity)
				fbr.swapBase(prevBase)
				fbr.swapActive(prevFn)

				// return result but catch relevant signals
				switch exc {
				case returnSignal:
					return result, nil
				default:
					return result, exc
				}
			}
		}

		// try go func
		if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
			return func(fbr *fiber) (Value, *Exception) {
				return fn.call(fbr, arguments)
			}
		}

		// try method
		if m, isMethod := value.asMethod(); isMethod {
			return func(fbr *fiber) (Value, *Exception) {
				return m.call(fbr, arguments)
			}
		}

		panic("cannot call whatever this is")
	}

	// optimise: calling methods (avoids heap allocation of Method{})
	if iFA, ok := node.Fn.(ast.FieldAccess); ok {
		if lhs, ok := vm.evaluate(iFA.Lhs).(local); ok {
			index := fields.Get(iFA.Rhs)
			return func(fbr *fiber) (Value, *Exception) {
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
				if value == nil {
					return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", iFA.Rhs, iFA)
				}
				return Method{this: obj, fn: *value}.call(fbr, arguments)
			}
		}
	}

	// generic compilation
	value := vm.compile(node.Fn)
	return func(fbr *fiber) (result Value, exc *Exception) {
		value, exc := value(fbr)
		if exc != nil {
			return value, exc
		}

		// check if it is a user function
		if fn, isUserFn := value.AsUserFn(); isUserFn {
			if len(fn.args) != len(arguments) {
				if fn.name != "λ" {
					return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
				}
				return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(arguments))
			}

			// setup stack locals
			base := len(fbr.stack)
			for idx := range fn.capacity {
				fbr.stack = append(fbr.stack, fbr.newValue())

				// evaluate arguments
				if idx < len(arguments) {
					arg, exc := arguments[idx](fbr)
					if exc != nil {
						return arg, exc
					}

					*(fbr.stack[base+idx]) = arg
				}
			}

			// save currently captured values
			prevBase := fbr.swapBase(base)
			prevFn := fbr.swapActive(fn)

			// correctly invoke the function
			switch {
			// sync -> unsync
			case !fbr.unsynced && fn.mode == ast.UnsyncedMode:
				vm.rt.ReleaseGIL()
				fbr.unsynced = true
				result, exc = fn.code(fbr)
				fbr.unsynced = false
				vm.rt.AcquireGIL()

			// unsync -> sync
			case fbr.unsynced && fn.mode == ast.SyncedMode:
				vm.rt.AcquireGIL()
				fbr.unsynced = false
				result, exc = fn.code(fbr)
				fbr.unsynced = true
				vm.rt.ReleaseGIL()

			// others
			default:
				result, exc = fn.code(fbr)
			}

			// release non-escaping locals
			for _, idx := range fn.recyclable {
				fbr.putValue(fbr.stack[base+idx])
			}

			// restore old state
			fbr.popStack(fn.capacity)
			fbr.swapBase(prevBase)
			fbr.swapActive(prevFn)

			// return result but catch relevant signals
			switch exc {
			case returnSignal:
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

/*
DO NOT MODIFY WITHOUT CAUSE!
I had to sacrifice 3 goats and a chicken in order to discover cache invalidation issues.
Look at the `NOTE` below.
*/
func (vm *Instance) emitGo(node ast.Go) instruction {
	if node, isCall := node.Fn.(ast.Call); isCall {
		// compile arguments
		arguments := make([]instruction, len(node.Args))
		for i, arg := range node.Args {
			arguments[i] = vm.compile(arg)
		}

		// generic compilation
		value := vm.compile(node.Fn)
		return func(fbr *fiber) (result Value, exc *Exception) {
			value, exc := value(fbr)
			if exc != nil {
				return value, exc
			}

			// check if it is a user function
			if fn, isUserFn := value.AsUserFn(); isUserFn {
				if len(fn.args) != len(arguments) {
					if fn.name != "λ" {
						return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
					}
					return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(arguments))
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

				// setup mode
				var unsynced bool
				switch fn.mode {
				case ast.UnsyncedMode:
					unsynced = true
				case ast.SyncedMode:
					unsynced = false
				case ast.InheritMode:
					unsynced = fbr.unsynced
				}

				task := make(chan evaluation, 1)
				vm.rt.wg.Go(func() {
					// setup new fiber
					fbr := vm.newFiber()
					fbr.active = fn
					fbr.base = 0
					fbr.stack = fbr.stack[:0]
					fbr.unsynced = unsynced

					// setup stack locals
					for idx := range fn.capacity {
						fbr.stack = append(fbr.stack, fbr.newValue())

						// NOTE: len(params) INSTEAD OF len(arguments) OR YOU WILL LOSE HAIR
						// assign arguments
						if idx < len(params) {
							*(fbr.stack[idx]) = params[idx]
						}
					}

					// run code
					if fbr.unsynced {
						result, exc = fn.code(fbr)
					} else {
						vm.rt.AcquireGIL()
						result, exc = fn.code(fbr)
						vm.rt.ReleaseGIL()
					}

					// release non-escaping locals
					for _, idx := range fn.recyclable {
						fbr.putValue(fbr.stack[idx])
					}
					fbr.popStack(fn.capacity)

					// return result but catch relevant signals
					switch exc {
					case nil:
					case returnSignal:
						exc = nil
					default:
						panic(exc)
					}
					task <- evaluation{result: result, err: exc}
				})

				return BoxTask(task), nil
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
			return func(fbr *fiber) (Value, *Exception) {
				return value, returnSignal
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
					return func(fbr *fiber) (Value, *Exception) {
						return fbr.getCaptured(v.index), returnSignal
					}
				}
				return func(fbr *fiber) (Value, *Exception) {
					return fbr.getLocal(v.index), returnSignal
				}
			}
		}
	}

	what := vm.compile(node.Value)
	return func(fbr *fiber) (Value, *Exception) {
		v, err := what(fbr)
		if err != nil {
			return v, err
		}

		return v, returnSignal
	}
}

func (vm *Instance) emitAwait(node ast.Await) instruction {
	task := vm.compile(node.Task)

	return func(fbr *fiber) (Value, *Exception) {
		value, err := task(fbr)
		if err != nil {
			return value, err
		}

		if task, isTask := value.AsTask(); isTask {
			if !fbr.unsynced {
				vm.rt.ReleaseGIL()
			}

			response, ok := <-task

			if !fbr.unsynced {
				vm.rt.AcquireGIL()
			}

			if !ok {
				return Value{}, CustomError("cannot await on a finished task")
			}

			return response.result, response.err
		}
		return Value{}, CustomError("cannot await on a value of type '%s'", value.TypeOf())
	}
}

func (vm *Instance) emitAwaitAll(node ast.AwaitAll) instruction {
	tasks := make([]instruction, len(node.Tasks))
	for i, task := range node.Tasks {
		tasks[i] = vm.compile(task)
	}

	return func(fbr *fiber) (Value, *Exception) {
		values := make([]<-chan evaluation, len(tasks))
		for i, task := range tasks {
			v, exc := task(fbr)
			if exc != nil {
				return v, exc

			}

			if task, ok := v.AsTask(); ok {
				values[i] = task
				continue
			}

			return Value{}, RuntimeExceptionF("cannot await on a value of type '%s'", v.TypeOf())
		}

		results := make([]Value, len(values))

		if !fbr.unsynced {
			vm.rt.ReleaseGIL()
		}

		for i, value := range values {
			response, ok := <-value

			if !ok {
				return Value{}, CustomError("cannot await on a finished task")
			}

			if response.err != nil {
				return response.result, response.err
			}

			results[i] = response.result
		}

		if !fbr.unsynced {
			vm.rt.AcquireGIL()
		}

		return BoxArray(results), nil
	}
}

func (vm *Instance) emitConditional(node ast.Conditional) instruction {
	condition := vm.compile(node.Condition)
	action := vm.compile(node.Action)

	if node.Otherwise != nil {
		otherwise := vm.compile(node.Otherwise)

		return func(fbr *fiber) (Value, *Exception) {
			v, err := condition(fbr)
			if err != nil {
				return v, err
			}

			if v.IsTruthy() {
				return action(fbr)
			}
			return otherwise(fbr)
		}
	}

	return func(fbr *fiber) (Value, *Exception) {
		v, err := condition(fbr)
		if err != nil {
			return v, err
		}

		if v.IsTruthy() {
			return action(fbr)
		}
		return Value{}, nil
	}
}

func (vm *Instance) emitWhile(node ast.While) instruction {
	condition := vm.compile(node.Condition)
	action := vm.compile(node.Action)

	return func(fbr *fiber) (Value, *Exception) {
		for {
			// evaluate condition
			v, err := condition(fbr)
			if err != nil {
				return v, err
			}

			if !v.IsTruthy() {
				break
			}

			// evaluate action
			v, err = action(fbr)
			if err != nil {
				if err == continueSignal {
					continue
				} else if err == breakSignal {
					break
				}
				return v, err
			}
		}
		return Value{}, nil
	}
}

func (vm *Instance) emitBlock(node ast.Block) instruction {
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
				return func(fbr *fiber) (Value, *Exception) {
					return value, returnSignal
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
						return func(fbr *fiber) (Value, *Exception) {
							return fbr.getCaptured(v.index), returnSignal
						}
					}
					return func(fbr *fiber) (Value, *Exception) {
						return fbr.getLocal(v.index), returnSignal
					}
				}
			}

			what := vm.compile(ret.Value)
			return func(fbr *fiber) (Value, *Exception) {
				v, err := what(fbr)
				if err != nil {
					return v, err
				}

				return v, returnSignal
			}
		}

		// generic
		return vm.compile(node)
	}

	block := make([]instruction, len(node.Code))
	for i, statement := range node.Code {
		block[i] = vm.compile(statement)
	}

	return func(fbr *fiber) (Value, *Exception) {
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
			return func(fbr *fiber) (Value, *Exception) {
				v := fbr.get(lhs)
				if field, exists := v.getField(index); exists {
					return field, nil
				}
				return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", node.Rhs, node)
			}

		case Value:
			// I don't think this gets used tbh because using vm.evaluate(ast.FieldAccess) bypasses this
			if field, exists := lhs.getField(index); exists {
				return func(fbr *fiber) (Value, *Exception) {
					return field, nil
				}
			}
			panic(TypeErrorF("undefined symbol '%v' in '%v'", node.Rhs, node))

		case Global:
			// global non-static binding
			return func(fbr *fiber) (Value, *Exception) {
				if field, exists := lhs.getField(index); exists {
					return field, nil
				}
				panic(TypeErrorF("undefined symbol '%v' in '%v'", node.Rhs, node))
			}
		}
	}

	// generic compilation
	lhs := vm.compile(node.Lhs)
	return func(fbr *fiber) (Value, *Exception) {
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

func (vm *Instance) emitNeg(node ast.Neg) instruction {
	value := vm.compile(node.Value)

	return func(fbr *fiber) (Value, *Exception) {
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
						return func(fbr *fiber) (Value, *Exception) {
							lhs, rhs := fbr.get(lhs), fbr.get(rhs)
							if result, ok := lhs.Add(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("+", lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs, rhs := fbr.get(lhs), fbr.get(rhs)
							if result, ok := lhs.Sub(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("-", lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs, rhs := fbr.get(lhs), fbr.get(rhs)
							if result, ok := lhs.Mul(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("*", lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs, rhs := fbr.get(lhs), fbr.get(rhs)
							if result, ok := lhs.Mul(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("/", lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs, rhs := fbr.get(lhs), fbr.get(rhs)
							if result, ok := lhs.Mod(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("%", lhs, rhs)
						}

					case ast.EqOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs, rhs := fbr.get(lhs), fbr.get(rhs)
							return BoxBool(lhs.Equals(rhs)), nil
						}

					case ast.LtOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs, rhs := fbr.get(lhs), fbr.get(rhs)
							if result, ok := lhs.LessThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.GtOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs, rhs := fbr.get(lhs), fbr.get(rhs)
							if result, ok := lhs.GreaterThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError(">", lhs, rhs)
						}

					case ast.LtEqOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs, rhs := fbr.get(lhs), fbr.get(rhs)
							if result, ok := lhs.LessThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<=", lhs, rhs)
						}

					case ast.GtEqOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs, rhs := fbr.get(lhs), fbr.get(rhs)
							if result, ok := lhs.GreaterThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError(">=", lhs, rhs)
						}

					case ast.OrOp:
						return func(fbr *fiber) (Value, *Exception) {
							if fbr.get(lhs).IsTruthy() {
								return BoxBool(true), nil
							} else if fbr.get(rhs).IsTruthy() {
								return BoxBool(true), nil
							}
							return BoxBool(false), nil
						}

					case ast.AndOp:
						return func(fbr *fiber) (Value, *Exception) {
							return BoxBool(fbr.get(lhs).IsTruthy() && fbr.get(rhs).IsTruthy()), nil
						}
					}
				}

				// optimise: rhs being a constant
				if rhs, isValue := rhs.(Value); isValue {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Add(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("+", lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Sub(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("-", lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Mul(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("*", lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Div(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("/", lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Div(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("%", lhs, rhs)
						}

					case ast.EqOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.get(lhs)
							return BoxBool(lhs.Equals(rhs)), nil
						}

					case ast.LtOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.LessThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.GtOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.GreaterThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.LtEqOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.LessThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<=", lhs, rhs)
						}

					case ast.GtEqOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.get(lhs)
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
						return func(fbr *fiber) (Value, *Exception) {
							rhs := fbr.get(rhs)
							if result, ok := lhs.Add(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("+", lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *fiber) (Value, *Exception) {
							rhs := fbr.get(rhs)
							if result, ok := lhs.Sub(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("-", lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *fiber) (Value, *Exception) {
							rhs := fbr.get(rhs)
							if result, ok := lhs.Mul(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("*", lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *fiber) (Value, *Exception) {
							rhs := fbr.get(rhs)
							if result, ok := lhs.Div(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("/", lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *fiber) (Value, *Exception) {
							rhs := fbr.get(rhs)
							if result, ok := lhs.Div(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("%", lhs, rhs)
						}

					case ast.EqOp:
						return func(fbr *fiber) (Value, *Exception) {
							rhs := fbr.get(rhs)
							return BoxBool(lhs.Equals(rhs)), nil
						}

					case ast.LtOp:
						return func(fbr *fiber) (Value, *Exception) {
							rhs := fbr.get(rhs)
							if result, ok := lhs.LessThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.GtOp:
						return func(fbr *fiber) (Value, *Exception) {
							rhs := fbr.get(rhs)
							if result, ok := lhs.GreaterThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.LtEqOp:
						return func(fbr *fiber) (Value, *Exception) {
							rhs := fbr.get(rhs)
							if result, ok := lhs.LessThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<=", lhs, rhs)
						}

					case ast.GtEqOp:
						return func(fbr *fiber) (Value, *Exception) {
							rhs := fbr.get(rhs)
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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

	case ast.LtOp:
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		if rhs := vm.evaluate(node.Rhs); rhs != nil {
			// optimise: lhs being a local
			if lhs, ok := lhs.(local); ok {
				if lhs.isStatic {
					panic(fmt.Sprintf("Assignment to constant binding on line '%v'.", node.Line()))
				}

				// optimise: rhs being a local
				if rhs, ok := rhs.(local); ok {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.getByRef(lhs)
							rhs := fbr.get(rhs)
							if result, ok := lhs.Add(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("+", *lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.getByRef(lhs)
							rhs := fbr.get(rhs)
							if result, ok := lhs.Sub(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("-", *lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.getByRef(lhs)
							rhs := fbr.get(rhs)
							if result, ok := lhs.Mul(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("*", *lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.getByRef(lhs)
							rhs := fbr.get(rhs)
							if result, ok := lhs.Div(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("/", *lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.getByRef(lhs)
							rhs := fbr.get(rhs)
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
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.getByRef(lhs)
							if result, ok := lhs.Add(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("+", *lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.getByRef(lhs)
							if result, ok := lhs.Sub(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("-", *lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.getByRef(lhs)
							if result, ok := lhs.Mul(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("*", *lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.getByRef(lhs)
							if result, ok := lhs.Div(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("/", *lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *fiber) (Value, *Exception) {
							lhs := fbr.getByRef(lhs)
							if result, ok := lhs.Mod(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("%", *lhs, rhs)
						}
					}
				}
			}
		}
	}

	/* lhs := vm.compile(node.Lhs)
	rhs := vm.compile(node.Rhs)

	// generic compilation
	switch node.Operator {
	case ast.AddOp:
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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
		return func(fbr *fiber) (Value, *Exception) {
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

	panic(fmt.Errorf("implement Operator(%v)", node.Operator))
}
