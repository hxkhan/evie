package core

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/hk-32/evie/box"
	"github.com/hk-32/evie/internal/op"
)

// order should be consistent with ast/op.go
var instructions [op.NUM_OPS]func(rt *Routine) (box.Value, error)

var runs [op.NUM_OPS]int

var mathErrFormat = "operator '%v' expects numbers, got '%v' and '%v'"

func init() {
	instructions = [...]func(rt *Routine) (box.Value, error){
		func(rt *Routine) (box.Value, error) { // NULL
			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // EXIT
			fmt.Printf("EXIT at ip %v \n", rt.ip)
			os.Exit(0)
			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // ECHO
			v, err := rt.next()
			if err != nil {
				return v, err
			}

			fmt.Println(Stringify(v))
			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // INT
			v := *(*int64)(unsafe.Pointer(&rt.code[rt.ip+1]))
			rt.ip += 8
			return box.Int64(v), nil
		},
		func(rt *Routine) (box.Value, error) { // FLOAT
			v := *(*float64)(unsafe.Pointer(&rt.code[rt.ip+1]))
			rt.ip += 8
			return box.Float64(v), nil
		},
		func(rt *Routine) (box.Value, error) { // STR
			size := int(*(*uint16)(unsafe.Pointer(&rt.code[rt.ip+1])))
			str := unsafe.String(&rt.code[rt.ip+3], size)
			rt.ip += 2 + size
			return box.String(str), nil
		},
		func(rt *Routine) (box.Value, error) { // TRUE
			return box.Bool(true), nil
		},
		func(rt *Routine) (box.Value, error) { // FALSE
			return box.Bool(false), nil
		},
		func(rt *Routine) (box.Value, error) { // ADD
			a, err := rt.next()
			if err != nil {
				return a, err
			}
			b, err := rt.next()
			if err != nil {
				return a, err
			}

			if a, ok := a.AsInt64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Int64(a + b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Float64(float64(a) + b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Float64(a + float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Float64(a + b), nil
				}
			}

			return box.Value{}, OperatorTypesError("+", a, b)
		},
		func(rt *Routine) (box.Value, error) { // SUB
			a, err := rt.next()
			if err != nil {
				return a, err
			}

			b, err := rt.next()
			if err != nil {
				return a, err
			}

			if a, ok := a.AsInt64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Int64(a - b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Float64(float64(a) - b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Float64(a - float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Float64(a - b), nil
				}
			}

			return box.Value{}, OperatorTypesError("-", a, b)
		},
		func(rt *Routine) (box.Value, error) { // MUL
			a, err := rt.next()
			if err != nil {
				return a, err
			}
			b, err := rt.next()
			if err != nil {
				return a, err
			}

			if a, ok := a.AsInt64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Int64(a * b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Float64(float64(a) * b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Float64(a * float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Float64(a * b), nil
				}
			}

			return box.Value{}, OperatorTypesError("*", a, b)
		},
		func(rt *Routine) (box.Value, error) { // DIV
			a, err := rt.next()
			if err != nil {
				return a, err
			}
			b, err := rt.next()
			if err != nil {
				return a, err
			}

			if a, ok := a.AsInt64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Float64(float64(a) / float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Float64(float64(a) / b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Float64(a / float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Float64(a / b), nil
				}
			}

			return box.Value{}, OperatorTypesError("/", a, b)
		},
		func(rt *Routine) (box.Value, error) { // NEG
			o, err := rt.next()
			if err != nil {
				return o, err
			}

			if a, ok := o.AsInt64(); ok {
				return box.Int64(-a), nil
			}

			if a, ok := o.AsFloat64(); ok {
				return box.Float64(-a), nil
			}

			return box.Value{}, CustomError("negation not supported on '%v'", Stringify(o))
		},
		func(rt *Routine) (box.Value, error) { // EQ
			a, err := rt.next()
			if err != nil {
				return a, err
			}
			b, err := rt.next()
			if err != nil {
				return a, err
			}
			return box.Bool(a == b), nil
		},
		func(rt *Routine) (box.Value, error) { // LS
			a, err := rt.next()
			if err != nil {
				return a, err
			}
			b, err := rt.next()
			if err != nil {
				return a, err
			}

			if a, ok := a.AsInt64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Bool(a < b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Bool(float64(a) < b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Bool(a < float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Bool(a < b), nil
				}
			}

			return box.Value{}, OperatorTypesError("<", a, b)
		},
		func(rt *Routine) (box.Value, error) { // MR
			a, err := rt.next()
			if err != nil {
				return a, err
			}
			b, err := rt.next()
			if err != nil {
				return a, err
			}

			if a, ok := a.AsInt64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Bool(a > b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Bool(float64(a) > b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return box.Bool(a > float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return box.Bool(a > b), nil
				}
			}

			return box.Value{}, OperatorTypesError(">", a, b)
		},
		func(rt *Routine) (box.Value, error) { // IF
		IF:
			size := int(uint16(rt.code[rt.ip+1]) | uint16(rt.code[rt.ip+2])<<8)
			jmp := rt.ip + size

			rt.ip += 2
			v, err := rt.next()
			if err != nil {
				return v, err
			}

			if !v.IsTruthy() {
				// this would make rt.ip point to an op.ELIF or op.ELSE or op.END
				rt.ip = jmp
				if rt.code[rt.ip] == op.ELIF {
					goto IF
				} else if rt.code[rt.ip] == op.ELSE {
					rt.ip += 2
				}
				// we are sure rt.ip is pointing at an op.ELSE or op.END
				return box.Value{}, nil
			}
			// Basically fallthrough the byte rt.code to the true section...
			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // ELIF (runaway) so skip
		AGAIN:
			// The last op.IF/ELIF was successful, so skip all remaining op.ELIF's and a potential op.ELSE
			// this would make rt.ip point to an op.ELIF or op.ELSE or op.END; former 2 need to be skipped
			rt.ip += int(rt.code[rt.ip+1])
			if rt.code[rt.ip] == op.ELIF || rt.code[rt.ip] == op.ELSE {
				goto AGAIN
			}
			// we are sure rt.ip is pointing at an op.END
			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // ELSE (runaway) so skip
			// The last op.IF/op.ELIF was successful, so skip this op.ELSE
			// this would make rt.ip point to an op.END
			rt.ip += int(rt.code[rt.ip+1])
			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // END
			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // LOAD_BUILTIN
			rt.ip++
			index := int(rt.code[rt.ip])
			return rt.m.builtins[index], nil
		},
		func(rt *Routine) (box.Value, error) { // LOAD_LOCAL
			rt.ip++
			index := int(rt.code[rt.ip])
			return *rt.active[rt.getCurrentBase()+index], nil
		},
		func(rt *Routine) (box.Value, error) { // STORE_LOCAL
			rt.ip++
			index := int(rt.code[rt.ip])

			v, err := rt.next()
			if err != nil {
				return v, err
			}

			rt.storeLocal(index, v)
			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // LOAD_CAPTURED
			rt.ip++
			index := int(rt.code[rt.ip])
			return rt.getCaptured(index), nil
		},
		func(rt *Routine) (box.Value, error) { // STORE_CAPTURED
			index := int(rt.code[rt.ip+1])
			rt.ip += 1

			v, err := rt.next()
			if err != nil {
				return v, err
			}

			rt.storeCaptured(index, v)
			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // FN_DECL
			info := rt.m.funcs[rt.ip]
			index := int(rt.code[rt.ip+1])
			captured := make([]*box.Value, len(info.Refs))
			for i, ref := range info.Refs {
				base := rt.getScrolledBase(ref.Scroll)
				captured[i] = rt.active[base+ref.Index]
			}

			*rt.active[rt.getCurrentBase()+index] = box.UserFn(unsafe.Pointer(&fn{captured, info}))
			rt.ip = info.End
			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // LAMBDA
			info := rt.m.funcs[rt.ip]
			captured := make([]*box.Value, len(info.Refs))
			for i, ref := range info.Refs {
				base := rt.getScrolledBase(ref.Scroll)
				captured[i] = rt.active[base+ref.Index]
			}
			rt.ip = info.End
			return box.UserFn(unsafe.Pointer(&fn{captured, info})), nil
		},
		func(rt *Routine) (box.Value, error) { // CALL
			rt.ip++
			nargsProvided := int(rt.code[rt.ip])
			start := rt.ip + 1
			value, err := rt.next()
			if err != nil {
				return value, err
			}

			// check if its a user function
			if ptr, isUserFn := value.AsUserFn(); isUserFn {
				fn := *(*fn)(ptr)

				if len(fn.Args) != nargsProvided {
					if fn.Name != "λ" {
						return box.Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.Name, len(fn.Args), nargsProvided)
					}
					return box.Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.Args), nargsProvided)
				}

				// create space for all the locals
				base := len(rt.active)
				for range fn.Capacity {
					rt.active = append(rt.active, boxPool.Get())
				}

				// set arguments
				for i := range nargsProvided {
					v, err := rt.next()
					if err != nil {
						return v, err
					}
					*rt.active[base+i] = v
				}
				rt.pushBase(base)

				// return to caller context
				retAddr, retEnc := rt.ip, rt.captured

				// reset
				var value box.Value
				var err error

				rt.captured = fn.captured
				for rt.ip = fn.Start; rt.ip < fn.End; rt.ip++ {
					value, err = instructions[rt.code[rt.ip]](rt)
					if err != nil {
						if err == errReturnSignal {
							err = nil
							break
						}
						// prep call stack trace
						rt.m.trace = append(rt.m.trace, fn.Name)
						break
					}
				}

				// release non-escaping locals
				for _, index := range fn.NonEscaping {
					boxPool.Put(rt.active[base+index])
				}

				rt.exitUserFN(retAddr, fn.Capacity, retEnc)
				return value, err
			} /* else if res, err := rt.tryNativeCall(value, nargsProvided); err != errNotFunction {
				return res, err
			} */

			// nothing more can be done; throw error
			switch rt.code[start] {
			case op.LOAD_LOCAL, op.LOAD_CAPTURED, op.LOAD_BUILTIN:
				return box.Value{}, CustomError("cannot call '%v', a non-function '%v'", rt.m.references[start], Stringify(value))
			}
			return box.Value{}, CustomError("cannot call a non-function '%v'", Stringify(value))
		},
		func(rt *Routine) (box.Value, error) { // RET
			v, err := rt.next()
			if err != nil {
				return v, err
			}
			// no error
			return v, errReturnSignal
		},
		func(rt *Routine) (box.Value, error) { // GO
			panic("implement go")
			/* rt.ip++
			nargsProvided := int(rt.code[rt.ip])
			start := rt.ip + 1
			value, err := rt.next()
			if err != nil {
				return value, err
			}

			// check if its a user function
			if ptr, isUserFn := value.AsUserFn(); isUserFn {
				fn := *(*fn)(ptr)

				if len(fn.Args) != nargsProvided {
					if fn.Name != "λ" {
						return box.Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.Name, len(fn.Args), nargsProvided)
					}
					return box.Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.Args), nargsProvided)
				}

				// evaluate and store locals
				locals := make([]*box.Value, fn.Capacity)
				for i := range fn.Capacity {
					locals[i] = boxPool.Get()
				}

				for i := range nargsProvided {
					v, err := rt.next()
					if err != nil {
						return v, err
					}
					*locals[i] = v
				}

				go func(rt *Routine) {
					rt.AcquireGIL()

					for rt.ip < fn.End {
						_, err := instructions[rt.code[rt.ip]](rt)
						if err != nil {
							if err != errReturnSignal {
								fmt.Println(err)
								os.Exit(0)
							}
							break
						}
						rt.ip++
					}

					// release non-escaping locals
					for _, index := range fn.NonEscaping {
						rt.m.pool.Put(rt.active[rt.getBase()+index])
					}

					rt.terminate()
				}(rt.newRoutine(fn.Start, locals, fn.captured))
				return box.Value{}, nil
			} else if value.TypeOf() == "function" {
				return box.Value{}, CustomError("go on native functions is not supported")
			}

			// nothing more can be done; throw error
			switch rt.code[start] {
			case op.LOAD_LOCAL, op.LOAD_CAPTURED, op.LOAD_BUILTIN:
				return box.Value{}, CustomError("go on '%v', a non-function '%v' of type '%v'.", rt.m.references[start], Stringify(value), value.TypeOf())
			}
			return box.Value{}, CustomError("go on a non-function '%v' of type '%v'.", Stringify(value), value.TypeOf()) */
		},
		func(rt *Routine) (box.Value, error) { // AWAIT
			panic("implement await")
			/* value, err := rt.next()
			if err != nil {
				return value, err
			}

			if task, isTask := value.(Task); isTask {
				rt.ReleaseGIL()
				response, ok := <-task
				rt.AcquireGIL()

				if !ok {
					return box.Value{}, CustomError("cannot await on a closed task")
				}

				return response.Value, response.Error
			}
			return box.Value{}, CustomError("cannot await on '%v' of type '%v'", Stringify(value), value.TypeOf()) */
		},
		func(rt *Routine) (box.Value, error) { // AWAIT_ALL
			panic("implement await_all")
			/* rt.ip++
			nargs := int(rt.code[rt.ip])

			tuple := make(Tuple, nargs)

			for i := range nargs {
				value, err := rt.next()
				if err != nil {
					return value, err
				}

				if task, isTask := value.(Task); isTask {
					rt.ReleaseGIL()
					response, ok := <-task
					rt.AcquireGIL()

					if !ok {
						return box.Value{}, CustomError("cannot await on a discontinued task")
					}

					// NOTE: figure out what to do with the rest of the tasks. is it correct to leave them be?
					if response.Error != nil {
						return response.Value, response.Error
					}

					tuple[i] = response.Value
					continue
				}
				return box.Value{}, CustomError("cannot await on variable '%v' with a value of type '%v'", rt.m.references[rt.ip], value.TypeOf())
			}

			return tuple, nil */
		},
		func(rt *Routine) (box.Value, error) { // AWAIT_ANY
			panic("implement await_any")
			/* rt.ip++
			nargs := rt.code[rt.ip]

			cases := make([]reflect.SelectCase, nargs)
			for i := range nargs {
				value, err := rt.next()
				if err != nil {
					return value, err
				}

				task, isTask := value.(Task)
				if !isTask {
					return box.Value{}, CustomError("cannot await on variable '%v' with a value of type '%v'", rt.m.references[rt.ip], value.TypeOf())
				}

				cases[i] = reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(task),
				}
			}

			rt.ReleaseGIL()
			chosen, v, ok := reflect.Select(cases)
			rt.AcquireGIL()

			if !ok {
				return box.Value{}, CustomError("cannot await on a discontinued task")
			}

			response := v.Interface().(TaskResult)
			if response.Error != nil {
				return response.Value, response.Error
			}

			return Tuple{response.Value, int64(chosen)}, nil */
		},
		func(rt *Routine) (box.Value, error) { // LOOP
			v, err := rt.next()
			if err != nil {
				return v, err
			}
			// no error
			return v, errReturnSignal
		},

		func(rt *Routine) (box.Value, error) { // INC
			rt.ip++
			OP := rt.code[rt.ip]
			rt.ip++
			index := int(rt.code[rt.ip])

			var value box.Value
			switch OP {
			case op.LOAD_LOCAL:
				value = rt.getLocal(index)
			case op.LOAD_CAPTURED:
				value = rt.getCaptured(index)
			}

			if f, isFloat64 := value.AsFloat64(); isFloat64 {
				value = box.Float64(f + 1)
			} else if i, isInt64 := value.AsInt64(); isInt64 {
				value = box.Int64(i + 1)
			} else {
				return box.Value{}, CustomError("cannot increment variable '%v' with a value of type '%v'", rt.m.references[rt.ip], value.TypeOf())
			}

			switch OP {
			case op.LOAD_LOCAL:
				rt.storeLocal(index, value)
			case op.LOAD_CAPTURED:
				rt.storeCaptured(index, value)
			}
			return value, nil
		},
		func(rt *Routine) (box.Value, error) { // DEC
			rt.ip++
			OP := rt.code[rt.ip]
			rt.ip++
			index := int(rt.code[rt.ip])

			var value box.Value
			switch OP {
			case op.LOAD_LOCAL:
				value = rt.getLocal(index)
			case op.LOAD_CAPTURED:
				value = rt.getCaptured(index)
			}

			if f, isFloat64 := value.AsFloat64(); isFloat64 {
				value = box.Float64(f - 1)
			} else if i, isInt64 := value.AsInt64(); isInt64 {
				value = box.Int64(i - 1)
			} else {
				return box.Value{}, CustomError("cannot decremenent variable '%v' with a value of type '%v'", rt.m.references[rt.ip], value.TypeOf())
			}

			switch OP {
			case op.LOAD_LOCAL:
				rt.storeLocal(index, value)
			case op.LOAD_CAPTURED:
				rt.storeCaptured(index, value)
			}
			return value, nil
		},
		func(rt *Routine) (box.Value, error) { // STORE_ADD
			rt.ip++
			OP := rt.code[rt.ip]
			rt.ip++
			index := int(rt.code[rt.ip])

			var left box.Value
			switch OP {
			case op.LOAD_LOCAL:
				left = rt.getLocal(index)
			case op.LOAD_CAPTURED:
				left = rt.getCaptured(index)
			}

			right, err := rt.next()
			if err != nil {
				return right, err
			}

			if a, ok := left.AsInt64(); ok {
				if b, ok := right.AsInt64(); ok {
					left = box.Int64(a + b)
					goto SAVE
				} else if b, ok := right.AsFloat64(); ok {
					left = box.Float64(float64(a) + b)
					goto SAVE
				}
			} else if a, ok := left.AsFloat64(); ok {
				if b, ok := right.AsInt64(); ok {
					left = box.Float64(a + float64(b))
					goto SAVE
				} else if b, ok := right.AsFloat64(); ok {
					left = box.Float64(a + b)
					goto SAVE
				}
			}

			return box.Value{}, CustomError(mathErrFormat, "+", left, right)

		SAVE:
			switch OP {
			case op.LOAD_LOCAL:
				rt.storeLocal(index, left)
			case op.LOAD_CAPTURED:
				rt.storeCaptured(index, left)
			}

			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // STORE_SUB
			rt.ip++
			OP := rt.code[rt.ip]
			rt.ip++
			index := int(rt.code[rt.ip])

			var left box.Value
			switch OP {
			case op.LOAD_LOCAL:
				left = *rt.active[rt.getCurrentBase()+int(index)]
			case op.LOAD_CAPTURED:
				left = *rt.captured[index]
			}

			right, err := rt.next()
			if err != nil {
				return right, err
			}

			if a, ok := left.AsInt64(); ok {
				if b, ok := right.AsInt64(); ok {
					left = box.Int64(a - b)
					goto SAVE
				} else if b, ok := right.AsFloat64(); ok {
					left = box.Float64(float64(a) - b)
					goto SAVE
				}
			} else if a, ok := left.AsFloat64(); ok {
				if b, ok := right.AsInt64(); ok {
					left = box.Float64(a - float64(b))
					goto SAVE
				} else if b, ok := right.AsFloat64(); ok {
					left = box.Float64(a - b)
					goto SAVE
				}
			}

			return box.Value{}, CustomError(mathErrFormat, "-", left, right)

		SAVE:
			switch OP {
			case op.LOAD_LOCAL:
				rt.storeLocal(index, left)
			case op.LOAD_CAPTURED:
				rt.storeCaptured(index, left)
			}

			return box.Value{}, nil
		},
		func(rt *Routine) (box.Value, error) { // ADD_RIGHT_CONST
			a, err := rt.next()
			if err != nil {
				return a, err
			}

			bOp := rt.code[rt.ip+1]
			b := unsafe.Pointer(&rt.code[rt.ip+2])
			rt.ip += 9

			if a, ok := a.AsInt64(); ok {
				if bOp == op.INT {
					return box.Int64(a + *(*int64)(b)), nil
				}
				return box.Float64(float64(a) + *(*float64)(b)), nil
			}
			if a, ok := a.AsFloat64(); ok {
				if bOp == op.INT {
					return box.Float64(a + float64(*(*int64)(b))), nil
				}
				return box.Float64(a + *(*float64)(b)), nil
			}

			return box.Value{}, CustomError(mathErrFormat, "+", a, b)
		},
		func(rt *Routine) (box.Value, error) { // SUB_RIGHT_CONST
			a, err := rt.next()
			if err != nil {
				return a, err
			}

			bOp := rt.code[rt.ip+1]
			b := unsafe.Pointer(&rt.code[rt.ip+2])
			rt.ip += 9

			if a, ok := a.AsInt64(); ok {
				if bOp == op.INT {
					return box.Int64(a - *(*int64)(b)), nil
				}
				return box.Float64(float64(a) - *(*float64)(b)), nil
			}
			if a, ok := a.AsFloat64(); ok {
				if bOp == op.INT {
					return box.Float64(a - float64(*(*int64)(b))), nil
				}
				return box.Float64(a - *(*float64)(b)), nil
			}

			return box.Value{}, CustomError(mathErrFormat, "-", a, b)
		},
		func(rt *Routine) (box.Value, error) { // LS_RIGHT_CONST
			a, err := rt.next()
			if err != nil {
				return a, err
			}

			bOp := rt.code[rt.ip+1]
			b := unsafe.Pointer(&rt.code[rt.ip+2])
			rt.ip += 9

			if a, ok := a.AsInt64(); ok {
				if bOp == op.INT {
					return box.Bool(a < *(*int64)(b)), nil
				}
				return box.Bool(float64(a) < *(*float64)(b)), nil
			}
			if a, ok := a.AsFloat64(); ok {
				if bOp == op.INT {
					return box.Bool(a < float64(*(*int64)(b))), nil
				}
				return box.Bool(a < *(*float64)(b)), nil
			}

			return box.Value{}, CustomError(mathErrFormat, "<", a, b)
		},
		func(rt *Routine) (box.Value, error) { // RETURN_IF
			size := int(rt.code[rt.ip+1])
			jmp := rt.ip + size

			rt.ip += 1
			v, err := rt.next()
			if err != nil {
				return v, err
			}

			if v.IsTruthy() {
				v, err := rt.next()
				if err != nil {
					return v, err
				}
				// no error
				return v, errReturnSignal
			}
			// this would make rt.ip point to an op.END
			rt.ip = jmp
			return box.Value{}, nil
		},
	}
}

func (rt *Routine) Start() (box.Value, error) {
	rt.m.gil.Lock()

	for rt.ip = 0; rt.ip < len(rt.code); rt.ip++ {
		// fetch the instruction
		if v, err := instructions[rt.code[rt.ip]](rt); err != nil {
			if err == errReturnSignal {
				return v, nil
			}
			return v, errWithTrace{err, rt.m.trace}
		}
	}

	ptr, _ := (*rt.active[rt.m.entry]).AsUserFn()
	main := *(*fn)(ptr)
	rt.captured = main.captured
	//rt.active = rt.active[:0]

	// create space for locals of main
	rt.pushBase(len(rt.active))
	for range main.Capacity {
		rt.active = append(rt.active, boxPool.Get())
	}

	// run main
	for rt.ip = main.Start; rt.ip < main.End; rt.ip++ {
		if v, err := instructions[rt.code[rt.ip]](rt); err != nil {
			if err == errReturnSignal {
				return v, nil
			}
			return v, errWithTrace{err, rt.m.trace}
		}
	}

	// release non-escaping locals
	for _, index := range main.NonEscaping {
		boxPool.Put(rt.active[rt.getCurrentBase()+index])
	}

	rt.popLocals(main.Capacity)
	rt.popBase()

	rt.m.gil.Unlock()
	rt.m.wg.Wait()

	//fmt.Printf("LOCALS: LEN(%v) CAP(%v) \n", len(rt.locals), cap(rt.locals))
	//fmt.Printf("PUSHES(%v) \n", PUSHES)
	//fmt.Printf("BASIS: LEN(%v) CAP(%v) \n", len(m.basis), cap(m.basis))
	return box.Value{}, nil
}

// evaluates the next value and returns it
func (rt *Routine) next() (box.Value, error) {
	rt.ip++
	return instructions[rt.code[rt.ip]](rt)
}

// evaluates the next value and returns it, panics on errors
func (rt *Routine) nextP() box.Value {
	rt.ip++
	v, err := instructions[rt.code[rt.ip]](rt)
	if err != nil {
		panic(err)
	}
	return v
}

func (rt *Routine) exitUserFN(oldAddr int, nLocals int, oldEnc []*box.Value) {
	// return to caller context
	rt.ip = oldAddr
	rt.popLocals(nLocals)
	rt.popBase()
	rt.captured = oldEnc
}

func (rt *Routine) tryNativeCall(value any, nargsP int) (result box.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = box.Value{}
			err = r.(error)
		}
	}()

	switch nargsP {
	case 0:
		if fn, ok := value.(NativeFn[func() (box.Value, error)]); ok {
			return fn.Callable()
		}
	case 1:
		if fn, ok := value.(NativeFn[func(box.Value) (box.Value, error)]); ok {
			return fn.Callable(rt.nextP())
		}
	case 2:
		if fn, ok := value.(NativeFn[func(box.Value, box.Value) (box.Value, error)]); ok {
			return fn.Callable(rt.nextP(), rt.nextP())
		}
	case 3:
		if fn, ok := value.(NativeFn[func(box.Value, box.Value, box.Value) (box.Value, error)]); ok {
			return fn.Callable(rt.nextP(), rt.nextP(), rt.nextP())
		}
	}

	// check if its even a function
	/* if value.TypeOf() == "function" {
		fn := value.(interface {
			Name() string
			Nargs() int
		})

		if fn.Nargs() != nargsP {
			return box.Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.Name(), fn.Nargs(), nargsP)
		}

		panic("function, but not callable, what is this even??")
	} */

	return box.Value{}, errNotFunction
}
