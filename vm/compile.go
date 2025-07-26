package vm

import (
	"fmt"
	"log"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/vm/scope"
)

type instruction func(fbr *fiber) (Value, error)

func (vm *Instance) compile(node ast.Node) instruction {
	switch node := node.(type) {
	case ast.Package:
		vm.cp.pkg = vm.rt.packages[node.Name]
		if vm.cp.pkg == nil {
			vm.cp.pkg = &Package{symbols: make(map[string]Symbol)}
			vm.rt.packages[node.Name] = vm.cp.pkg
		}

		this := vm.cp.pkg

		/*
			------ Hoisting Protocol ------

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

		var code []instruction

		// 1. declare all symbols
		for _, node := range node.Code {
			if fn, isFn := node.(ast.Fn); isFn {
				if _, exists := this.symbols[fn.Name]; exists {
					panic(fmt.Errorf("double declaration of %s", fn.Name))
				}
				this.symbols[fn.Name] = Symbol{Value: &Value{}}
			} else if iDec, isIdentDec := node.(ast.IdentDec); isIdentDec {
				if _, exists := this.symbols[iDec.Name]; exists {
					panic(fmt.Errorf("double declaration of %s", iDec.Name))
				}
				this.symbols[iDec.Name] = Symbol{Value: &Value{}}
			}
		}

		// 2. physically move function initialization to the top
		for _, node := range node.Code {
			if fn, isFn := node.(ast.Fn); isFn {
				symbol := this.symbols[fn.Name]

				vm.cp.openClosure()
				vm.cp.scope = scope.NewScope(0)

				// declare the fn arguments and only then compile the code
				for _, arg := range fn.Args {
					vm.cp.scope.Declare(arg)
				}

				action := vm.compile(fn.Action)
				capacity := vm.cp.scope.Capacity()
				closure := vm.cp.closeClosure()

				// make list of non-escaping variables so they can be recycled after execution
				recyclable := make([]int, 0, capacity-closure.freeVars.Len())
				for index := range capacity {
					if closure.freeVars.Has(index) {
						log.Printf("CT: fn %v => Local(%v) escapes\n", fn.Name, index)
						continue
					}
					recyclable = append(recyclable, index)
				}

				info := &funcInfoStatic{
					name:       fn.Name,
					args:       fn.Args,
					recyclable: recyclable,
					capacity:   capacity,
					code:       action,
					vm:         vm,
				}

				code = append(code, func(fbr *fiber) (Value, error) {
					// store the function
					*symbol.Value = BoxUserFn(UserFn{funcInfoStatic: info})
					return Value{}, nil
				})
			}
		}

		// 3. compile the rest of the code
		for _, node := range node.Code {
			// skip functions now
			if _, isFn := node.(ast.Fn); isFn {
				continue
			}

			// compile global variable initialization in a special way because indices are pre declared
			if iDec, isIdentDec := node.(ast.IdentDec); isIdentDec {
				symbol := this.symbols[iDec.Name]
				value := vm.compile(iDec.Value)
				code = append(code, func(fbr *fiber) (Value, error) {
					v, err := value(fbr)
					if err != nil {
						return v, err
					}

					// store the value
					*symbol.Value = v
					return Value{}, nil
				})
				continue
			}

			// other code
			in := vm.compile(node)
			code = append(code, in)
		}

		return func(fbr *fiber) (Value, error) {
			for _, in := range code {
				if v, err := in(fbr); err != nil {
					return v, err
				}
			}
			return Value{}, nil
		}

	case ast.Input[bool]:
		value := BoxBool(node.Value)
		return func(fbr *fiber) (Value, error) {
			return value, nil
		}

	case ast.Input[float64]:
		value := BoxFloat64(node.Value)
		return func(fbr *fiber) (Value, error) {
			return value, nil
		}

	case ast.Input[string]:
		value := BoxString(node.Value)
		return func(fbr *fiber) (Value, error) {
			return value, nil
		}

	case ast.Input[struct{}]:
		return func(fbr *fiber) (Value, error) {
			return Value{}, nil
		}

	case ast.Echo:
		what := vm.compile(node.Value)

		return func(fbr *fiber) (Value, error) {
			v, err := what(fbr)
			if err != nil {
				return v, err
			}

			fmt.Println(v)
			return Value{}, nil
		}

	case ast.IdentDec:
		return vm.emitIdentDec(node)

	case ast.IdentGet:
		return vm.emitIdentGet(node)

	case ast.IdentSet:
		return vm.emitIdentSet(node)

	case ast.Block:
		return vm.emitBlock(node)

	case ast.Conditional:
		return vm.emitConditional(node)

	case ast.Fn:
		return vm.emitFn(node)

	case ast.Call:
		return vm.emitCall(node)
	case ast.DotCall:
		return vm.emitDotCall(node)

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

func (vm *Instance) emitIdentDec(node ast.IdentDec) instruction {
	index, success := vm.cp.scope.Declare(node.Name)
	if !success {
		panic(fmt.Errorf("double declaration of %s", node.Name))
	}

	value := vm.compile(node.Value)
	return func(fbr *fiber) (Value, error) {
		v, err := value(fbr)
		if err != nil {
			return v, err
		}

		fbr.storeLocal(local(index), v)
		return Value{}, nil
	}
}

func (vm *Instance) emitIdentGet(node ast.IdentGet) instruction {
	variable, err := vm.cp.reach(node.Name)
	if err != nil {
		panic(err)
	}

	switch v := variable.(type) {
	case local:
		return func(fbr *fiber) (Value, error) {
			return fbr.getLocal(v), nil
		}
	case captured:
		return func(fbr *fiber) (Value, error) {
			return fbr.getCaptured(v), nil
		}
	case *Value:
		return func(fbr *fiber) (Value, error) {
			return *v, nil
		}
	}

	panic("ayo what")
}

func (vm *Instance) emitIdentSet(node ast.IdentSet) instruction {
	variable, err := vm.cp.reach(node.Name)
	if err != nil {
		panic(err)
	}

	value := vm.compile(node.Value)

	switch v := variable.(type) {
	case local:
		return func(fbr *fiber) (Value, error) {
			value, err := value(fbr)
			if err != nil {
				return value, err
			}

			fbr.storeLocal(v, value)
			return Value{}, nil
		}

	case captured:
		return func(fbr *fiber) (Value, error) {
			value, err := value(fbr)
			if err != nil {
				return value, err
			}

			fbr.storeCaptured(v, value)
			return Value{}, nil
		}

	case *Value:
		return func(fbr *fiber) (Value, error) {
			value, err := value(fbr)
			if err != nil {
				return value, err
			}

			*v = value
			return Value{}, nil
		}
	}

	panic("ayo what")
}

func (vm *Instance) emitFn(node ast.Fn) instruction {
	if node.Name != "" {
		panic("named functions are only allowed as top level declarations")
	}

	vm.cp.openClosure()
	vm.cp.scope = vm.cp.scope.New(0)

	// declare the fn arguments and only then compile the code
	for _, arg := range node.Args {
		vm.cp.scope.Declare(arg)
	}

	code := vm.compile(node.Action)

	capacity := vm.cp.scope.Capacity()
	closure := vm.cp.closeClosure()
	vm.cp.scope = vm.cp.scope.Previous()

	// make list of non-escaping variables so they can be recycled after execution
	recyclable := make([]int, 0, capacity-closure.freeVars.Len())
	for index := range capacity {
		if closure.freeVars.Has(index) {
			log.Printf("CT: closure => Local(%v) escapes\n", index)
			continue
		}
		recyclable = append(recyclable, index)
	}

	info := &funcInfoStatic{
		name:       "λ",
		args:       node.Args,
		captures:   closure.captures,
		recyclable: recyclable,
		capacity:   capacity,
		code:       code,
		vm:         vm,
	}

	for i, ref := range closure.captures {
		log.Printf("CT: closure => Capture(%v) -> %v\n", i, ref)
	}

	return func(fbr *fiber) (Value, error) {
		captured := make([]*Value, len(closure.captures))
		for i, ref := range closure.captures {
			var v *Value
			if ref.isLocal {
				v = fbr.getLocalByRef(ref.index)
			} else {
				v = fbr.getCapturedByRef(ref.index)
			}
			captured[i] = v
			log.Printf("RT: closure => Capture(%v) -> %v -> %v\n", i, ref, v)
		}

		// create the user fn & return it
		fn := UserFn{
			funcInfoStatic: info,
			references:     captured,
		}
		return BoxUserFn(fn), nil
	}
}

func (vm *Instance) emitCall(node ast.Call) instruction {
	// compile arguments
	arguments := make([]instruction, len(node.Args))
	for i, arg := range node.Args {
		arguments[i] = vm.compile(arg)
	}

	// optimise: calling global & builtin functions
	if iGet, isIdentGet := node.Fn.(ast.IdentGet); isIdentGet && vm.cp.optimise {
		variable, err := vm.cp.reach(iGet.Name)
		if err != nil {
			panic(err)
		}

		switch value := variable.(type) {
		case *Value:
			return func(fbr *fiber) (result Value, err error) {
				// check if it is a user function
				if fn, isUserFn := value.AsUserFn(); isUserFn {
					if len(fn.args) != len(arguments) {
						if fn.name != "λ" {
							return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
						}
						return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(arguments))
					}

					// create space for all the locals
					base := len(fbr.stack)
					for range fn.capacity {
						fbr.stack = append(fbr.stack, vm.newValue())
					}

					// evaluate arguments & push them on the stack
					for idx, argument := range arguments {
						v, err := argument(fbr)
						if err != nil {
							return v, err
						}

						*(fbr.stack[base+idx]) = v
					}

					// prep for execution & save currently captured values
					prevBase := fbr.swapBase(base)
					prevFn := fbr.swapActive(fn)
					result, err = fn.code(fbr)

					// release non-escaping locals
					for _, idx := range fn.recyclable {
						vm.putValue(fbr.stack[base+idx])
					}

					// restore old state
					fbr.popStack(fn.capacity)
					fbr.swapBase(prevBase)
					fbr.swapActive(prevFn)

					// don't implicitly return the return value of the last executed instruction
					switch err {
					case nil:
						return Value{}, nil
					case errReturnSignal:
						return result, nil
					default:
						return result, err
					}
				}

				return Value{}, CustomError("cannot call a non-function '%v'", *value)
			}
		}
	}

	// generic compilation
	value := vm.compile(node.Fn)
	return func(fbr *fiber) (result Value, err error) {
		value, err := value(fbr)
		if err != nil {
			return value, err
		}

		// check if it is a user function
		if fn, isUserFn := value.AsUserFn(); isUserFn {
			if len(fn.args) != len(arguments) {
				if fn.name != "λ" {
					return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(arguments))
				}
				return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(arguments))
			}

			// create space for all the locals
			base := len(fbr.stack)
			for range fn.capacity {
				fbr.stack = append(fbr.stack, vm.newValue())
			}

			// evaluate arguments & push them on the stack
			for idx, argument := range arguments {
				v, err := argument(fbr)
				if err != nil {
					return v, err
				}

				*(fbr.stack[base+idx]) = v
			}

			// prep for execution & save currently captured values
			prevBase := fbr.swapBase(base)
			prevFn := fbr.swapActive(fn)
			result, err = fn.code(fbr)

			// release non-escaping locals
			for _, idx := range fn.recyclable {
				vm.putValue(fbr.stack[base+idx])
			}

			// restore old state
			fbr.popStack(fn.capacity)
			fbr.swapBase(prevBase)
			fbr.swapActive(prevFn)

			// don't implicitly return the return value of the last executed instruction
			switch err {
			case nil:
				return Value{}, nil
			case errReturnSignal:
				return result, nil
			default:
				return result, err
			}
		} else if res, err := fbr.tryNativeCall(value, arguments); err != errNotFunction {
			return res, err
		}

		return Value{}, CustomError("cannot call a non-function '%v'", value)
	}
}

func (vm *Instance) emitDotCall(node ast.DotCall) instruction {
	// namespaces e.g. json.decode(...)
	iGetLeft, isLeftIdentGet := node.Left.(ast.IdentGet)
	iGetRight, isRightIdentGet := node.Right.(ast.IdentGet)
	if isLeftIdentGet && isRightIdentGet {
		name := iGetLeft.Name + "." + iGetRight.Name

		if _, exists := vm.opts.Builtins[name]; exists {
			return vm.compile(ast.Call{Pos: node.Pos, Fn: ast.IdentGet{Pos: node.Pos, Name: name}, Args: node.Args})
		} else {
			panic("method not found")
		}
	}

	panic("implement the rest")
}

func (vm *Instance) emitGo(node ast.Go) instruction {
	if node, isCall := node.Fn.(ast.Call); isCall {
		call := vm.emitCall(node)

		return func(fbr *fiber) (Value, error) {
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
	if vm.cp.optimise {
		// optimise: returning constants
		if in, isInput := node.Value.(ast.Input[float64]); isInput {
			value := BoxFloat64(in.Value)
			return func(fbr *fiber) (Value, error) {
				return value, errReturnSignal
			}
		}

		// optimise: returning local variables
		if iGet, isIdentGet := node.Value.(ast.IdentGet); isIdentGet {
			variable, err := vm.cp.reach(iGet.Name)
			if err != nil {
				panic(err)
			}

			if idx, isLocal := variable.(local); isLocal {
				return func(fbr *fiber) (Value, error) {
					return fbr.getLocal(idx), errReturnSignal
				}
			}
		}
	}

	what := vm.compile(node.Value)
	return func(fbr *fiber) (Value, error) {
		v, err := what(fbr)
		if err != nil {
			return v, err
		}

		return v, errReturnSignal
	}
}

func (vm *Instance) emitAwait(node ast.Await) instruction {
	task := vm.compile(node.Task)

	return func(fbr *fiber) (Value, error) {
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

		return func(fbr *fiber) (Value, error) {
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

	return func(fbr *fiber) (Value, error) {
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

func (vm *Instance) emitBlock(node ast.Block) instruction {
	vm.cp.scope.OpenBlock()
	defer vm.cp.scope.CloseBlock()

	// optimise: statement extraction from block; saves an extra dispatch
	if len(node.Code) == 1 && vm.cp.optimise {
		node := node.Code[0]
		// optimise: {return x}
		if ret, isReturn := node.(ast.Return); isReturn {
			// optimise: returning constants
			if in, isInput := ret.Value.(ast.Input[float64]); isInput {
				value := BoxFloat64(in.Value)
				return func(fbr *fiber) (Value, error) {
					return value, errReturnSignal
				}
			}

			// optimise: returning local variables
			if iGet, isIdentGet := ret.Value.(ast.IdentGet); isIdentGet {
				variable, err := vm.cp.reach(iGet.Name)
				if err != nil {
					panic(err)
				}

				if idx, isLocal := variable.(local); isLocal {
					return func(fbr *fiber) (Value, error) {
						return fbr.getLocal(idx), errReturnSignal
					}
				}
			}

			what := vm.compile(ret.Value)
			return func(fbr *fiber) (Value, error) {
				v, err := what(fbr)
				if err != nil {
					return v, err
				}

				return v, errReturnSignal
			}
		}

		// generic
		return vm.compile(node)
	}

	block := make([]instruction, len(node.Code))
	for i, statement := range node.Code {
		block[i] = vm.compile(statement)
	}

	return func(fbr *fiber) (Value, error) {
		for _, statement := range block {
			if v, err := statement(fbr); err != nil {
				return v, err
			}
		}
		return Value{}, nil
	}
}

func (vm *Instance) emitBinOp(node ast.BinOp) instruction {
	// optimise: lhs being a local variable
	if iGet, isIdentGet := node.Lhs.(ast.IdentGet); isIdentGet && vm.cp.optimise {
		variable, err := vm.cp.reach(iGet.Name)
		if err != nil {
			panic(err)
		}

		switch v := variable.(type) {
		case local:
			// optimise: rhs being a constant
			if rhs, isInput := node.Rhs.(ast.Input[float64]); isInput {
				switch node.Operator {
				case ast.AddOp:
					return func(fbr *fiber) (Value, error) {
						a := fbr.getLocal(v)
						if a, ok := a.AsFloat64(); ok {
							return BoxFloat64(a + rhs.Value), nil
						}
						return Value{}, operatorError("+", a, rhs.Value)
					}

				case ast.SubOp:
					return func(fbr *fiber) (Value, error) {
						a := fbr.getLocal(v)
						if a, ok := a.AsFloat64(); ok {
							return BoxFloat64(a - rhs.Value), nil
						}
						return Value{}, operatorError("-", a, rhs.Value)
					}

				case ast.LtOp:
					return func(fbr *fiber) (Value, error) {
						a := fbr.getLocal(v)
						if a, ok := a.AsFloat64(); ok {
							return BoxBool(a < rhs.Value), nil
						}
						return Value{}, operatorError("<", a, rhs.Value)
					}
				}
			}

			// generic rhs
			rhs := vm.compile(node.Rhs)
			switch node.Operator {
			case ast.AddOp:
				return func(fbr *fiber) (Value, error) {
					a := fbr.getLocal(v)
					b, err := rhs(fbr)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsFloat64(); ok {
						if b, ok := b.AsFloat64(); ok {
							return BoxFloat64(a + b), nil
						}
					}
					return Value{}, operatorError("+", a, b)
				}
			case ast.SubOp:
				return func(fbr *fiber) (Value, error) {
					a := fbr.getLocal(v)
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
				return func(fbr *fiber) (Value, error) {
					a := fbr.getLocal(v)
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
	if rhs, isInput := node.Rhs.(ast.Input[float64]); isInput && vm.cp.optimise {
		switch node.Operator {
		case ast.AddOp:
			return func(fbr *fiber) (Value, error) {
				lhs, err := lhs(fbr)
				if err != nil {
					return lhs, err
				}

				if a, ok := lhs.AsFloat64(); ok {
					return BoxFloat64(a + float64(rhs.Value)), nil
				}
				return Value{}, operatorError("+", lhs, rhs)
			}

		case ast.SubOp:
			return func(fbr *fiber) (Value, error) {
				lhs, err := lhs(fbr)
				if err != nil {
					return lhs, err
				}

				if a, ok := lhs.AsFloat64(); ok {
					return BoxFloat64(a - float64(rhs.Value)), nil
				}
				return Value{}, operatorError("-", lhs, rhs)
			}

		case ast.LtOp:
			return func(fbr *fiber) (Value, error) {
				lhs, err := lhs(fbr)
				if err != nil {
					return lhs, err
				}

				if a, ok := lhs.AsFloat64(); ok {
					return BoxBool(a < float64(rhs.Value)), nil
				}
				return Value{}, operatorError("<", lhs, rhs)
			}
		}
	}

	rhs := vm.compile(node.Rhs)

	// generic compilation
	switch node.Operator {
	case ast.AddOp:
		return func(fbr *fiber) (Value, error) {
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

			return Value{}, operatorError("+", a, b)
		}

	case ast.SubOp:
		return func(fbr *fiber) (Value, error) {
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
		return func(fbr *fiber) (Value, error) {
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
		return func(fbr *fiber) (Value, error) {
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
		return func(fbr *fiber) (Value, error) {
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
		return func(fbr *fiber) (Value, error) {
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
		return func(fbr *fiber) (Value, error) {
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
