package vm

import (
	"fmt"
	"log"

	"github.com/hxkhan/evie/ast"
)

type instruction func(fbr *fiber) (Value, error)

func (vm *Instance) compile(node ast.Node) instruction {
	switch node := node.(type) {
	case ast.Package:
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
				idx, success := vm.cp.scope.Declare(fn.Name)
				if !success {
					panic(fmt.Errorf("double declaration of %s", fn.Name))
				}
				vm.cp.globals[fn.Name] = idx
			}

			if iDec, isIdentDec := node.(ast.IdentDec); isIdentDec {
				vm.cp.uninitializedGlobals[iDec.Name] = struct{}{}
				idx, _ := vm.cp.scope.Declare(iDec.Name)
				vm.cp.globals[iDec.Name] = idx
			}
		}

		// 2. physically move function initialization to the top
		for _, node := range node.Code {
			if fn, isFn := node.(ast.Fn); isFn {
				idx := vm.cp.globals[fn.Name]

				vm.cp.openClosure()
				vm.cp.scope = vm.cp.scope.New()

				// declare the fn arguments and only then compile the code
				for _, arg := range fn.Args {
					vm.cp.scope.Declare(arg)
				}

				action := vm.compile(fn.Action)
				capacity := vm.cp.scope.Capacity()
				closure := vm.cp.closeClosure()
				vm.cp.scope = vm.cp.scope.Previous()

				// make list of non-escaping variables so they can be freed after execution
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
					captures:   closure.captures,
					recyclable: recyclable,
					capacity:   capacity,
					code:       action,
					vm:         vm,
				}

				for i, ref := range closure.captures {
					log.Printf("CT: fn %v => Capture(%v) -> %v\n", fn.Name, i, ref)
				}

				code = append(code, func(fbr *fiber) (Value, error) {
					captured := make([]*Value, len(closure.captures))
					for i, ref := range closure.captures {
						var v *Value
						if ref.isLocal {
							v = fbr.getLocalByRef(ref.index)
						} else {
							v = fbr.getCapturedByRef(ref.index)
						}
						captured[i] = v
						log.Printf("RT: fn %v => Capture(%v) -> %v -> %v\n", fn.Name, i, ref, v)
					}

					fn := UserFn{
						funcInfoStatic: info,
						references:     captured,
					}

					// declare the function locally
					fbr.storeLocal(idx, BoxUserFn(fn))
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
				idx := vm.cp.globals[iDec.Name]
				value := vm.compile(iDec.Value)
				code = append(code, func(fbr *fiber) (Value, error) {
					v, err := value(fbr)
					if err != nil {
						return v, err
					}

					fbr.storeLocal(idx, v)
					return Value{}, nil
				})
				delete(vm.cp.uninitializedGlobals, iDec.Name)
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

			fbr.storeLocal(index, v)
			return Value{}, nil
		}

	case ast.IdentGet:
		ref, err := vm.cp.reach(node.Name)
		if err != nil {
			panic(err)
		}

		switch {
		case ref.isBuiltin():
			return func(fbr *fiber) (Value, error) {
				return vm.rt.builtins[ref.index], nil
			}
		case ref.isLocal():
			return func(fbr *fiber) (Value, error) {
				return fbr.getLocal(ref.index), nil
			}

		case ref.isCaptured():
			index := vm.cp.addToCaptured(ref)
			return func(fbr *fiber) (Value, error) {
				return fbr.getCaptured(index), nil
			}
		}

	case ast.IdentSet:
		ref, err := vm.cp.reach(node.Name)
		if err != nil {
			panic(err)
		}

		value := vm.compile(node.Value)
		switch {
		case ref.isBuiltin():
			panic("cannot set the value of a built-in")

		case ref.isLocal():
			return func(fbr *fiber) (Value, error) {
				v, err := value(fbr)
				if err != nil {
					return v, err
				}

				fbr.storeLocal(ref.index, v)
				return Value{}, nil
			}

		case ref.isCaptured():
			index := vm.cp.addToCaptured(ref)
			return func(fbr *fiber) (Value, error) {
				v, err := value(fbr)
				if err != nil {
					return v, err
				}

				fbr.storeCaptured(index, v)
				return Value{}, nil
			}
		}

	case ast.Block:
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
					ref, err := vm.cp.reach(iGet.Name)
					if err != nil {
						panic(err)
					}

					if ref.isLocal() {
						return func(fbr *fiber) (Value, error) {
							return fbr.getLocal(ref.index), errReturnSignal
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

	case ast.Conditional:
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

	case ast.Fn:
		if node.Name != "" {
			panic("named functions are only allowed as top level declarations")
		}

		vm.cp.openClosure()
		vm.cp.scope = vm.cp.scope.New()

		// declare the fn arguments and only then compile the code
		for _, arg := range node.Args {
			vm.cp.scope.Declare(arg)
		}

		code := vm.compile(node.Action)

		capacity := vm.cp.scope.Capacity()
		closure := vm.cp.closeClosure()
		vm.cp.scope = vm.cp.scope.Previous()

		// make list of non-escaping variables so they can be freed after execution
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

	case ast.Call:
		// compile arguments
		argsFetchers := make([]instruction, len(node.Args))
		for i, arg := range node.Args {
			argsFetchers[i] = vm.compile(arg)
		}

		// optimise: calling captured functions
		if iGet, isIdentGet := node.Fn.(ast.IdentGet); isIdentGet && vm.cp.optimise {
			ref, err := vm.cp.reach(iGet.Name)
			if err != nil {
				panic(err)
			}

			if ref.isCaptured() {
				index := vm.cp.addToCaptured(ref)
				return func(fbr *fiber) (result Value, err error) {
					value := fbr.getCaptured(index)

					// check if its a user function
					if fn, isUserFn := value.AsUserFn(); isUserFn {
						if len(fn.args) != len(argsFetchers) {
							if fn.name != "λ" {
								return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(argsFetchers))
							}
							return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(argsFetchers))
						}

						// create space for all the locals
						base := fbr.stackSize()
						for range fn.capacity {
							fbr.pushLocal(vm.newValue())
						}

						// set arguments
						for idx, fetcher := range argsFetchers {
							v, err := fetcher(fbr)
							if err != nil {
								return v, err
							}
							*fbr.stack[base+idx] = v
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
						fbr.popLocals(fn.capacity)
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

					return Value{}, CustomError("cannot call a non-function '%v'", value)
				}
			}
		}

		// generic compilation
		fnFetcher := vm.compile(node.Fn)
		return func(fbr *fiber) (result Value, err error) {
			value, err := fnFetcher(fbr)
			if err != nil {
				return value, err
			}

			// check if its a user function
			if fn, isUserFn := value.AsUserFn(); isUserFn {
				if len(fn.args) != len(argsFetchers) {
					if fn.name != "λ" {
						return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.name, len(fn.args), len(argsFetchers))
					}
					return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.args), len(argsFetchers))
				}

				// create space for all the locals
				base := fbr.stackSize()
				for range fn.capacity {
					fbr.pushLocal(vm.newValue())
				}

				// set arguments
				for idx, fetcher := range argsFetchers {
					v, err := fetcher(fbr)
					if err != nil {
						return v, err
					}
					*fbr.stack[base+idx] = v
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
				fbr.popLocals(fn.capacity)
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

			return Value{}, CustomError("cannot call a non-function '%v'", value)
		}

	case ast.Return:
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
				ref, err := vm.cp.reach(iGet.Name)
				if err != nil {
					panic(err)
				}

				if ref.isLocal() {
					return func(fbr *fiber) (Value, error) {
						return fbr.getLocal(ref.index), errReturnSignal
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

	case ast.BinOp:
		// optimise: lhs being a local variable
		if iGet, isIdentGet := node.Lhs.(ast.IdentGet); isIdentGet && vm.cp.optimise {
			ref, err := vm.cp.reach(iGet.Name)
			if err != nil {
				panic(err)
			}

			if ref.isLocal() {
				// optimise: rhs being a constant
				if rhs, isInput := node.Rhs.(ast.Input[float64]); isInput {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *fiber) (Value, error) {
							a := fbr.getLocal(ref.index)
							if a, ok := a.AsFloat64(); ok {
								return BoxFloat64(a + rhs.Value), nil
							}
							return Value{}, operatorError("+", a, rhs.Value)
						}

					case ast.SubOp:
						return func(fbr *fiber) (Value, error) {
							a := fbr.getLocal(ref.index)
							if a, ok := a.AsFloat64(); ok {
								return BoxFloat64(a - rhs.Value), nil
							}
							return Value{}, operatorError("-", a, rhs.Value)
						}

					case ast.LtOp:
						return func(fbr *fiber) (Value, error) {
							a := fbr.getLocal(ref.index)
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
						a := fbr.getLocal(ref.index)
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
						a := fbr.getLocal(ref.index)
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
						a := fbr.getLocal(ref.index)
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

	panic(fmt.Errorf("implement %T", node))
}

/* func encode(value any) Value {
	switch v := value.(type) {
	case bool:
		return BoxBool(v)
	case float64:
		return BoxFloat64(v)
	case string:
		return BoxString(v)
	}

	panic(fmt.Errorf("cannot encode %T", value))
}
*/
