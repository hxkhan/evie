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
		value := BoxFloat64(node.Value)
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

	case ast.BinOp:
		return vm.emitBinOp(node)
	}

	panic(fmt.Errorf("implement %T", node))
}

func (vm *Instance) runPackage(node ast.Package) (Value, *Exception) {
	vm.cp.pkg = vm.rt.packages[node.Name]
	if vm.cp.pkg == nil {
		vm.cp.pkg = &packageInstance{
			name:    node.Name,
			globals: map[int]Global{},
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
		------ Hoisting Protocol ------ (invalid since recent changes probably)

		All symbols are first symbolically pre-declared without initialization.
		This is so when we later initialize them; they can reference each other.
		Function initializations are physically moved to the top of the code.
		The rest of the code follows right after.

		So this is not possible because the order is maintained:
			x := y + 2
			y := 10

		But this is, because functions declarations are hoisted to the top:
			n := add(3, 7) // n <- 10

			fn square(a, b) {
				return a + b
			}
	*/

	// 1. declare all symbols
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
				vm:   vm,
			}})
			this.globals[index] = Global{Value: &fn, IsStatic: true}
		} else if iDec, isIdentDec := node.(ast.Decl); isIdentDec {
			index := fields.Get(iDec.Name)
			if _, exists := this.globals[index]; exists {
				panic(fmt.Errorf("double declaration of %s", iDec.Name))
			}
			this.globals[index] = Global{Value: &Value{}, IsStatic: iDec.IsStatic}
		}
	}

	// 2. initialize functions
	for _, node := range node.Code {
		if fn, isFn := node.(ast.Fn); isFn {
			global := this.globals[fields.Get(fn.Name)]
			ufn := (*UserFn)(global.pointer)

			vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, this: global.Value})
			vm.cp.closures.Last(0).scope.OpenBlock()

			// declare the fn arguments and only then compile the code
			for _, arg := range fn.Args {
				vm.cp.closures.Last(0).scope.Declare(arg)
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

		// compile global variable initialization in a special way because indices are pre declared
		if iDec, isIdentDec := node.(ast.Decl); isIdentDec {
			global := this.globals[fields.Get(iDec.Name)]
			value := vm.compile(iDec.Value)
			v, exc := value(vm.main)
			if exc != nil {
				return v, exc
			}
			// store the value
			*(global.Value) = v
			continue
		}
	}

	// 3. compile the rest of the code
	for _, node := range node.Code {
		// skip functions now
		if _, isFn := node.(ast.Fn); isFn {
			continue
		}

		// skip declarations now
		if _, isIdentDec := node.(ast.Decl); isIdentDec {
			continue
		}

		// other code
		v, exc := vm.compile(node)(vm.main)
		if exc != nil {
			return v, exc
		}
	}

	return Value{}, nil
}

func (vm *Instance) emitIdentDec(node ast.Decl) instruction {
	index, success := vm.cp.closures.Last(0).scope.Declare(node.Name)
	if !success {
		panic(fmt.Errorf("double declaration of %s", node.Name))
	}

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
				panic(fmt.Sprintf("Assignment to constant symbol '%v'.", iGet.Name))
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
		vm.cp.closures.Last(0).scope.Declare(arg)
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

	index, ok := vm.cp.closures.Last(0).scope.Declare(node.Name)
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
	if value, ok := vm.evaluate(node.Fn); ok {
		// try user fn
		if fn, isUserFn := value.AsUserFn(); isUserFn {
			if len(fn.args) != len(arguments) {
				if fn.name != "λ" {
					panic(CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments)))
				}
				panic(CustomError("function requires %v argument(s), %v provided", len(fn.args), len(arguments)))
			}

			// optimise: call to ourselves (recursion)
			if value == *(vm.cp.closures.Last(0).this) {
				return func(fbr *fiber) (result Value, exc *Exception) {
					// setup stack locals
					base := len(fbr.stack)
					for idx := range fn.capacity {
						fbr.stack = append(fbr.stack, vm.newValue())

						// evaluate arguments
						if idx < len(arguments) {
							arg, exc := arguments[idx](fbr)
							if exc != nil {
								return arg, exc
							}

							*(fbr.stack[base+idx]) = arg
						}
					}

					// prep for execution & save currently captured values
					prevBase := fbr.swapBase(base)
					result, exc = fn.code(fbr)

					// release non-escaping locals
					for _, idx := range fn.recyclable {
						vm.putValue(fbr.stack[base+idx])
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
					fbr.stack = append(fbr.stack, vm.newValue())

					// evaluate arguments
					if idx < len(arguments) {
						arg, exc := arguments[idx](fbr)
						if exc != nil {
							return arg, exc
						}

						*(fbr.stack[base+idx]) = arg
					}
				}

				// prep for execution & save currently captured values
				prevBase := fbr.swapBase(base)
				prevFn := fbr.swapActive(fn)
				result, exc = fn.code(fbr)

				// release non-escaping locals
				for _, idx := range fn.recyclable {
					vm.putValue(fbr.stack[base+idx])
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
				return fbr.call(fn, arguments)
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
				fbr.stack = append(fbr.stack, vm.newValue())

				// evaluate arguments
				if idx < len(arguments) {
					arg, exc := arguments[idx](fbr)
					if exc != nil {
						return arg, exc
					}

					*(fbr.stack[base+idx]) = arg
				}
			}

			// prep for execution & save currently captured values
			prevBase := fbr.swapBase(base)
			prevFn := fbr.swapActive(fn)
			result, exc = fn.code(fbr)

			// release non-escaping locals
			for _, idx := range fn.recyclable {
				vm.putValue(fbr.stack[base+idx])
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

		return fbr.tryNonStandardCall(value, arguments)
	}
}

func (vm *Instance) emitGo(node ast.Go) instruction {
	if node, isCall := node.Fn.(ast.Call); isCall {
		call := vm.emitCall(node)

		return func(fbr *fiber) (Value, *Exception) {
			vm.rt.wg.Add(1)

			go func(fbr *fiber) {
				vm.rt.gil.Lock()

				result, err := call(fbr)
				vm.rt.wg.Done()
				vm.rt.gil.Unlock()

				if err != nil {
					panic(err)
				}

				if result.IsTruthy() {
					fmt.Println("result:", result)
				}
			}(&fiber{active: fbr.active})
			return Value{}, nil
		}
	}
	panic("go expected call, got something else")
}

func (vm *Instance) emitReturn(node ast.Return) instruction {
	if vm.cp.inline {
		// optimise: returning constants
		if in, isInput := node.Value.(ast.Input[float64]); isInput {
			value := BoxFloat64(in.Value)
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
			vm.rt.gil.Unlock()
			response, ok := <-task
			vm.rt.gil.Lock()

			if !ok {
				return Value{}, CustomError("cannot await on a finished task")
			}

			return response.result, response.err
		}
		return Value{}, CustomError("cannot await on a value of type '%s'", value.TypeOf())
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
				value := BoxFloat64(in.Value)
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

	//if lhs, ok := vm.evaluate()

	// optimise: ident as lhs
	if iGet, isIdentGet := node.Lhs.(ast.Ident); isIdentGet && vm.cp.inline {
		variable, err := vm.cp.reach(iGet.Name)
		if err != nil {
			panic(err)
		}

		switch lhs := variable.(type) {
		case local:
			if lhs.isCaptured {
				return func(fbr *fiber) (Value, *Exception) {
					v := fbr.getCaptured(lhs.index)
					if field, exists := v.getField(index); exists {
						return field, nil
					}
					return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", node.Rhs, node)
				}
			}

			return func(fbr *fiber) (Value, *Exception) {
				v := fbr.getLocal(lhs.index)
				if field, exists := v.getField(index); exists {
					return field, nil
				}
				return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", node.Rhs, node)
			}

		case Global:
			if lhs.IsStatic {
				if field, exists := lhs.getField(index); exists {
					return func(fbr *fiber) (Value, *Exception) {
						return field, nil
					}
				}
				panic(TypeErrorF("undefined symbol '%v' in '%v'", node.Rhs, node))
			}
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

func (vm *Instance) emitBinOp(node ast.BinOp) instruction {
	// optimise: lhs being a local
	if iGet, isIdentGet := node.Lhs.(ast.Ident); isIdentGet && vm.cp.inline {
		variable, err := vm.cp.reach(iGet.Name)
		if err != nil {
			panic(err)
		}

		switch v := variable.(type) {
		case local:
			// optimise: rhs being a constant
			if input, isInput := node.Rhs.(ast.Input[float64]); isInput {
				rhs := input.Value

				switch node.Operator {
				case ast.AddOp:
					return func(fbr *fiber) (Value, *Exception) {
						lhs := fbr.get(v)
						if lhs, ok := lhs.AsFloat64(); ok {
							return BoxFloat64(lhs + rhs), nil
						}
						return Value{}, operatorError("+", lhs, BoxFloat64(rhs))
					}

				case ast.SubOp:
					return func(fbr *fiber) (Value, *Exception) {
						lhs := fbr.get(v)
						if lhs, ok := lhs.AsFloat64(); ok {
							return BoxFloat64(lhs - rhs), nil
						}
						return Value{}, operatorError("-", lhs, BoxFloat64(rhs))
					}

				case ast.LtOp:
					return func(fbr *fiber) (Value, *Exception) {
						lhs := fbr.get(v)
						if lhs, ok := lhs.AsFloat64(); ok {
							return BoxBool(lhs < rhs), nil
						}
						return Value{}, operatorError("<", lhs, BoxFloat64(rhs))
					}
				}
			}

			// generic rhs
			rhs := vm.compile(node.Rhs)
			switch node.Operator {
			case ast.AddOp:
				return func(fbr *fiber) (Value, *Exception) {
					a := fbr.get(v)
					b, err := rhs(fbr)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsFloat64(); ok {
						if b, ok := b.AsFloat64(); ok {
							return BoxFloat64(a + b), nil
						}
					}

					if a, ok := a.AsString(); ok {
						if b, ok := b.AsString(); ok {
							return BoxString(a + b), nil
						}
					}

					return Value{}, operatorError("+", a, b)
				}
			case ast.SubOp:
				return func(fbr *fiber) (Value, *Exception) {
					a := fbr.get(v)
					b, err := rhs(fbr)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsFloat64(); ok {
						if b, ok := b.AsFloat64(); ok {
							return BoxFloat64(a - b), nil
						}
					}
					return Value{}, operatorError("-", a, b)
				}

			case ast.LtOp:
				return func(fbr *fiber) (Value, *Exception) {
					a := fbr.get(v)
					b, err := rhs(fbr)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsFloat64(); ok {
						if b, ok := b.AsFloat64(); ok {
							return BoxBool(a < b), nil
						}
					}
					return Value{}, operatorError("<", a, b)
				}
			}
		}
	}

	lhs := vm.compile(node.Lhs)
	// optimise: rhs being a constant
	if rhs, isInput := node.Rhs.(ast.Input[float64]); isInput && vm.cp.inline {
		switch node.Operator {
		case ast.AddOp:
			return func(fbr *fiber) (Value, *Exception) {
				lhs, err := lhs(fbr)
				if err != nil {
					return lhs, err
				}

				if a, ok := lhs.AsFloat64(); ok {
					return BoxFloat64(a + float64(rhs.Value)), nil
				}
				return Value{}, operatorError("+", lhs, BoxFloat64(rhs.Value))
			}

		case ast.SubOp:
			return func(fbr *fiber) (Value, *Exception) {
				lhs, err := lhs(fbr)
				if err != nil {
					return lhs, err
				}

				if a, ok := lhs.AsFloat64(); ok {
					return BoxFloat64(a - float64(rhs.Value)), nil
				}
				return Value{}, operatorError("-", lhs, BoxFloat64(rhs.Value))
			}

		case ast.LtOp:
			return func(fbr *fiber) (Value, *Exception) {
				lhs, err := lhs(fbr)
				if err != nil {
					return lhs, err
				}

				if a, ok := lhs.AsFloat64(); ok {
					return BoxBool(a < float64(rhs.Value)), nil
				}
				return Value{}, operatorError("<", lhs, BoxFloat64(rhs.Value))
			}
		}
	}

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

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(a + b), nil
				}
			}

			if a, ok := a.AsString(); ok {
				if b, ok := b.AsString(); ok {
					return BoxString(a + b), nil
				}
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

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(a - b), nil
				}
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

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(a * b), nil
				}
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

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(a / b), nil
				}
			}

			return Value{}, operatorError("/", a, b)
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

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsFloat64(); ok {
					return BoxBool(a < b), nil
				}
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

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsFloat64(); ok {
					return BoxBool(a > b), nil
				}
			}

			return Value{}, operatorError(">", a, b)
		}
	}

	panic(fmt.Errorf("implement Operator(%v)", node.Operator))
}

// optimise: inline field access calls
/* 	if iFA, isFieldAccess := node.Fn.(ast.FieldAccess); isFieldAccess && vm.cp.inline {
	if iGet, isIdentGet := iFA.Lhs.(ast.Ident); isIdentGet && vm.cp.inline {
		index := fields.Get(iFA.Rhs)
		variable, err := vm.cp.reach(iGet.Name)
		if err != nil {
			panic(err)
		}

		switch lhs := variable.(type) {
		case local:
			return func(fbr *fiber) (Value, *Exception) {
				v := fbr.getLocal(lhs)
				field, exists := v.getField(index)
				if !exists {
					return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", iFA.Rhs, node)
				}

				// check if it is a user function
				if fn, isUserFn := field.AsUserFn(); isUserFn {
					if len(fn.args) != len(arguments) {
						if fn.name != "λ" {
							return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
						}
						return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(arguments))
					}

					// setup stack locals
					base := len(fbr.stack)
					for idx := range fn.capacity {
						fbr.stack = append(fbr.stack, vm.newValue())

						// evaluate arguments
						if idx < len(arguments) {
							arg, exc := arguments[idx](fbr)
							if exc != nil {
								return arg, exc
							}

							*(fbr.stack[base+idx]) = arg
						}
					}

					// prep for execution & save currently captured values
					prevBase := fbr.swapBase(base)
					prevFn := fbr.swapActive(fn)
					result, exc := fn.code(fbr)

					// release non-escaping locals
					for _, idx := range fn.recyclable {
						vm.putValue(fbr.stack[base+idx])
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

				return fbr.tryNonStandardCall(field, arguments)
			}

		case captured:
			return func(fbr *fiber) (Value, *Exception) {
				v := fbr.getCaptured(lhs)
				field, exists := v.getField(index)
				if !exists {
					return Value{}, RuntimeExceptionF("undefined symbol '%v' in '%v'", iFA.Rhs, node)
				}

				// check if it is a user function
				if fn, isUserFn := field.AsUserFn(); isUserFn {
					if len(fn.args) != len(arguments) {
						if fn.name != "λ" {
							return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
						}
						return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(arguments))
					}

					// setup stack locals
					base := len(fbr.stack)
					for idx := range fn.capacity {
						fbr.stack = append(fbr.stack, vm.newValue())

						// evaluate arguments
						if idx < len(arguments) {
							arg, exc := arguments[idx](fbr)
							if exc != nil {
								return arg, exc
							}

							*(fbr.stack[base+idx]) = arg
						}
					}

					// prep for execution & save currently captured values
					prevBase := fbr.swapBase(base)
					prevFn := fbr.swapActive(fn)
					result, exc := fn.code(fbr)

					// release non-escaping locals
					for _, idx := range fn.recyclable {
						vm.putValue(fbr.stack[base+idx])
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

				return fbr.tryNonStandardCall(field, arguments)
			}

		case Global:
			if lhs.IsStatic {
				field, exists := lhs.getField(index)
				if !exists {
					panic(TypeErrorF("undefined symbol '%v' in '%v'", iFA.Rhs, node))
				}

				return func(fbr *fiber) (Value, *Exception) {
					// check if it is a user function
					if fn, isUserFn := field.AsUserFn(); isUserFn {
						if len(fn.args) != len(arguments) {
							if fn.name != "λ" {
								return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
							}
							return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(arguments))
						}

						// setup stack locals
						base := len(fbr.stack)
						for idx := range fn.capacity {
							fbr.stack = append(fbr.stack, vm.newValue())

							// evaluate arguments
							if idx < len(arguments) {
								arg, exc := arguments[idx](fbr)
								if exc != nil {
									return arg, exc
								}

								*(fbr.stack[base+idx]) = arg
							}
						}

						// prep for execution & save currently captured values
						prevBase := fbr.swapBase(base)
						prevFn := fbr.swapActive(fn)
						result, exc := fn.code(fbr)

						// release non-escaping locals
						for _, idx := range fn.recyclable {
							vm.putValue(fbr.stack[base+idx])
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

					return fbr.tryNonStandardCall(field, arguments)
				}
			}
			return func(fbr *fiber) (Value, *Exception) {
				field, exists := lhs.getField(index)
				if !exists {
					panic(TypeErrorF("undefined symbol '%v' in '%v'", iFA.Rhs, node))
				}

				// check if it is a user function
				if fn, isUserFn := field.AsUserFn(); isUserFn {
					if len(fn.args) != len(arguments) {
						if fn.name != "λ" {
							return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
						}
						return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(arguments))
					}

					// setup stack locals
					base := len(fbr.stack)
					for idx := range fn.capacity {
						fbr.stack = append(fbr.stack, vm.newValue())

						// evaluate arguments
						if idx < len(arguments) {
							arg, exc := arguments[idx](fbr)
							if exc != nil {
								return arg, exc
							}

							*(fbr.stack[base+idx]) = arg
						}
					}

					// prep for execution & save currently captured values
					prevBase := fbr.swapBase(base)
					prevFn := fbr.swapActive(fn)
					result, exc := fn.code(fbr)

					// release non-escaping locals
					for _, idx := range fn.recyclable {
						vm.putValue(fbr.stack[base+idx])
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

				return fbr.tryNonStandardCall(field, arguments)
			}
		}
	}
}
*/
