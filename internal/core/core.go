package core

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/hk-32/evie/internal/op"
)

// order should be consistent with ast/op.go
var instructions [op.NUM_OPS]func(rt *CoRoutine) (Value, error)

var runs [op.NUM_OPS]int

var mathErrFormat = "operator '%v' expects numbers, got '%v' and '%v'"

func init() {
	instructions = [...]func(rt *CoRoutine) (Value, error){
		func(rt *CoRoutine) (Value, error) { // NULL
			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // EXIT
			fmt.Printf("EXIT at ip %v \n", rt.ip)
			os.Exit(0)
			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // ECHO
			v, err := rt.next()
			if err != nil {
				return v, err
			}

			fmt.Println(v)
			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // INT
			v := *(*int64)(unsafe.Pointer(&m.code[rt.ip+1]))
			rt.ip += 8
			return BoxInt64(v), nil
		},
		func(rt *CoRoutine) (Value, error) { // FLOAT
			v := *(*float64)(unsafe.Pointer(&m.code[rt.ip+1]))
			rt.ip += 8
			return BoxFloat64(v), nil
		},
		func(rt *CoRoutine) (Value, error) { // STR
			size := int(*(*uint16)(unsafe.Pointer(&m.code[rt.ip+1])))
			str := unsafe.String(&m.code[rt.ip+3], size)
			rt.ip += 2 + size
			return BoxString(str), nil
		},
		func(rt *CoRoutine) (Value, error) { // TRUE
			return BoxBool(true), nil
		},
		func(rt *CoRoutine) (Value, error) { // FALSE
			return BoxBool(false), nil
		},
		func(rt *CoRoutine) (Value, error) { // ADD
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
					return BoxInt64(a + b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(float64(a) + b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return BoxFloat64(a + float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(a + b), nil
				}
			}

			return Value{}, OperatorTypesError("+", a, b)
		},
		func(rt *CoRoutine) (Value, error) { // SUB
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
					return BoxInt64(a - b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(float64(a) - b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return BoxFloat64(a - float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(a - b), nil
				}
			}

			return Value{}, OperatorTypesError("-", a, b)
		},
		func(rt *CoRoutine) (Value, error) { // MUL
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
					return BoxInt64(a * b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(float64(a) * b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return BoxFloat64(a * float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(a * b), nil
				}
			}

			return Value{}, OperatorTypesError("*", a, b)
		},
		func(rt *CoRoutine) (Value, error) { // DIV
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
					return BoxFloat64(float64(a) / float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(float64(a) / b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return BoxFloat64(a / float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxFloat64(a / b), nil
				}
			}

			return Value{}, OperatorTypesError("/", a, b)
		},
		func(rt *CoRoutine) (Value, error) { // NEG
			o, err := rt.next()
			if err != nil {
				return o, err
			}

			if a, ok := o.AsInt64(); ok {
				return BoxInt64(-a), nil
			}

			if a, ok := o.AsFloat64(); ok {
				return BoxFloat64(-a), nil
			}

			return Value{}, CustomError("negation not supported on '%v'", o)
		},
		func(rt *CoRoutine) (Value, error) { // EQ
			a, err := rt.next()
			if err != nil {
				return a, err
			}
			b, err := rt.next()
			if err != nil {
				return a, err
			}
			return BoxBool(a == b), nil
		},
		func(rt *CoRoutine) (Value, error) { // LS
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
					return BoxBool(a < b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxBool(float64(a) < b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return BoxBool(a < float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxBool(a < b), nil
				}
			}

			return Value{}, OperatorTypesError("<", a, b)
		},
		func(rt *CoRoutine) (Value, error) { // MR
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
					return BoxBool(a > b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxBool(float64(a) > b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return BoxBool(a > float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return BoxBool(a > b), nil
				}
			}

			return Value{}, OperatorTypesError(">", a, b)
		},
		func(rt *CoRoutine) (Value, error) { // IF
		IF:
			size := int(uint16(m.code[rt.ip+1]) | uint16(m.code[rt.ip+2])<<8)
			jmp := rt.ip + size

			rt.ip += 2
			v, err := rt.next()
			if err != nil {
				return v, err
			}

			if !v.IsTruthy() {
				// this would make rt.ip point to an op.ELIF or op.ELSE or op.END
				rt.ip = jmp
				if m.code[rt.ip] == op.ELIF {
					goto IF
				} else if m.code[rt.ip] == op.ELSE {
					rt.ip += 2
				}
				// we are sure rt.ip is pointing at an op.ELSE or op.END
				return Value{}, nil
			}
			// Basically fallthrough the byte m.code to the true section...
			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // ELIF (runaway) so skip
		AGAIN:
			// The last op.IF/ELIF was successful, so skip all remaining op.ELIF's and a potential op.ELSE
			// this would make rt.ip point to an op.ELIF or op.ELSE or op.END; former 2 need to be skipped
			rt.ip += int(m.code[rt.ip+1])
			if m.code[rt.ip] == op.ELIF || m.code[rt.ip] == op.ELSE {
				goto AGAIN
			}
			// we are sure rt.ip is pointing at an op.END
			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // ELSE (runaway) so skip
			// The last op.IF/op.ELIF was successful, so skip this op.ELSE
			// this would make rt.ip point to an op.END
			rt.ip += int(m.code[rt.ip+1])
			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // END
			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // LOAD_BUILTIN
			rt.ip++
			index := int(m.code[rt.ip])
			return m.builtins[index], nil
		},
		func(rt *CoRoutine) (Value, error) { // LOAD_LOCAL
			rt.ip++
			index := int(m.code[rt.ip])
			return *rt.stack[rt.getCurrentBase()+index], nil
		},
		func(rt *CoRoutine) (Value, error) { // STORE_LOCAL
			rt.ip++
			index := int(m.code[rt.ip])

			v, err := rt.next()
			if err != nil {
				return v, err
			}

			rt.storeLocal(index, v)
			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // LOAD_CAPTURED
			rt.ip++
			index := int(m.code[rt.ip])
			return rt.getCaptured(index), nil
		},
		func(rt *CoRoutine) (Value, error) { // STORE_CAPTURED
			index := int(m.code[rt.ip+1])
			rt.ip += 1

			v, err := rt.next()
			if err != nil {
				return v, err
			}

			rt.storeCaptured(index, v)
			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // FN_DECL
			info := m.funcsMap[rt.ip]
			index := int(m.code[rt.ip+1])
			captured := make([]*Value, len(info.Refs))
			for i, ref := range info.Refs {
				captured[i] = rt.capture(ref.Index, ref.Scroll)
			}

			rt.storeLocal(index, BoxUserFn(UserFn{captured, info}))
			rt.ip = info.End
			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // LAMBDA
			info := m.funcsMap[rt.ip]
			captured := make([]*Value, len(info.Refs))
			for i, ref := range info.Refs {
				base := rt.getScrolledBase(ref.Scroll)
				captured[i] = rt.stack[base+ref.Index]
			}

			rt.ip = info.End
			return BoxUserFn(UserFn{captured, info}), nil
		},
		func(rt *CoRoutine) (Value, error) { // CALL
			rt.ip++
			nargsProvided := int(m.code[rt.ip])
			start := rt.ip + 1
			value, err := rt.next()
			if err != nil {
				return value, err
			}

			// check if its a user function
			if fn, isUserFn := value.AsUserFn(); isUserFn {

				if len(fn.Args) != nargsProvided {
					if fn.Name != "λ" {
						return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.Name, len(fn.Args), nargsProvided)
					}
					return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.Args), nargsProvided)
				}

				// create space for all the locals
				base := len(rt.stack)
				for range fn.Capacity {
					rt.stack = append(rt.stack, m.boxes.new())
				}

				// set arguments
				for i := range nargsProvided {
					v, err := rt.next()
					if err != nil {
						return v, err
					}
					*rt.stack[base+i] = v
				}
				rt.pushBase(base)

				// return to caller context
				retAddr, retEnc := rt.ip, rt.captured

				// reset
				var value Value
				var err error

				rt.captured = fn.captured
				for rt.ip = fn.Start; rt.ip < fn.End; rt.ip++ {
					value, err = instructions[m.code[rt.ip]](rt)
					if err != nil {
						if err == errReturnSignal {
							err = nil
							break
						}
						// prep call stack trace
						m.trace = append(m.trace, fn.Name)
						break
					}
				}

				// release non-escaping locals
				for _, index := range fn.NonEscaping {
					m.boxes.put(rt.stack[base+index])
				}

				rt.exitUserFN(retAddr, fn.Capacity, retEnc)
				return value, err
			} else if res, err := rt.tryNativeCall(value, nargsProvided); err != errNotFunction {
				return res, err
			}

			// nothing more can be done; throw error
			switch m.code[start] {
			case op.LOAD_LOCAL, op.LOAD_CAPTURED, op.LOAD_BUILTIN:
				return Value{}, CustomError("cannot call '%v', a non-function '%v'", m.symbolsMap[start], value)
			}
			return Value{}, CustomError("cannot call a non-function '%v'", value)
		},
		func(rt *CoRoutine) (Value, error) { // RET
			v, err := rt.next()
			if err != nil {
				return v, err
			}
			// no error
			return v, errReturnSignal
		},
		func(rt *CoRoutine) (Value, error) { // GO
			rt.ip++
			nargsProvided := int(m.code[rt.ip])
			start := rt.ip + 1
			value, err := rt.next()
			if err != nil {
				return value, err
			}

			// check if its a user function
			if fn, isUserFn := value.AsUserFn(); isUserFn {
				if len(fn.Args) != nargsProvided {
					if fn.Name != "λ" {
						return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.Name, len(fn.Args), nargsProvided)
					}
					return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.Args), nargsProvided)
				}

				// create space for all the locals
				locals := make([]*Value, fn.Capacity)
				for i := range fn.Capacity {
					locals[i] = m.boxes.new()
				}

				// set arguments
				for i := range nargsProvided {
					v, err := rt.next()
					if err != nil {
						return v, err
					}
					*locals[i] = v
				}

				m.wg.Add(1)
				go func(rt *CoRoutine) {
					m.gil.Lock()

					for rt.ip < fn.End {
						_, err := instructions[m.code[rt.ip]](rt)
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
						m.boxes.put(rt.stack[rt.getCurrentBase()+index])
					}

					m.wg.Done()
					m.gil.Unlock()
				}(rt.newRoutine(fn.Start, locals, fn.captured))
				return Value{}, nil
			} else if value.TypeOf() == "function" {
				return Value{}, CustomError("go on native functions is not supported")
			}

			// nothing more can be done; throw error
			switch m.code[start] {
			case op.LOAD_LOCAL, op.LOAD_CAPTURED, op.LOAD_BUILTIN:
				return Value{}, CustomError("go on '%s', a non-function '%s' of type '%v'.", m.symbolsMap[start], value.String(), value.TypeOf())
			}
			return Value{}, CustomError("go on a non-function '%s' of type '%s'.", value.String(), value.TypeOf())
		},
		func(rt *CoRoutine) (Value, error) { // AWAIT
			value, err := rt.next()
			if err != nil {
				return value, err
			}

			if task, isTask := value.AsTask(); isTask {
				m.gil.Unlock()
				response, ok := <-task
				m.gil.Lock()

				if !ok {
					return Value{}, CustomError("cannot await on a closed task")
				}

				return response.Result, response.Error
			}
			return Value{}, CustomError("cannot await on '%s' of type '%s'", value.String(), value.TypeOf())
		},
		func(rt *CoRoutine) (Value, error) { // AWAIT_ALL
			panic("implement await_all")
			/* rt.ip++
			nargs := int(m.code[rt.ip])

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
						return Value{}, CustomError("cannot await on a discontinued task")
					}

					// NOTE: figure out what to do with the rest of the tasks. is it correct to leave them be?
					if response.Error != nil {
						return response.Value, response.Error
					}

					tuple[i] = response.Value
					continue
				}
				return Value{}, CustomError("cannot await on variable '%v' with a value of type '%v'", rt.m.references[rt.ip], value.TypeOf())
			}

			return tuple, nil */
		},
		func(rt *CoRoutine) (Value, error) { // AWAIT_ANY
			panic("implement await_any")
			/* rt.ip++
			nargs := m.code[rt.ip]

			cases := make([]reflect.SelectCase, nargs)
			for i := range nargs {
				value, err := rt.next()
				if err != nil {
					return value, err
				}

				task, isTask := value.(Task)
				if !isTask {
					return Value{}, CustomError("cannot await on variable '%v' with a value of type '%v'", rt.m.references[rt.ip], value.TypeOf())
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
				return Value{}, CustomError("cannot await on a discontinued task")
			}

			response := v.Interface().(TaskResult)
			if response.Error != nil {
				return response.Value, response.Error
			}

			return Tuple{response.Value, int64(chosen)}, nil */
		},
		func(rt *CoRoutine) (Value, error) { // LOOP
			v, err := rt.next()
			if err != nil {
				return v, err
			}
			// no error
			return v, errReturnSignal
		},

		func(rt *CoRoutine) (Value, error) { // INC
			rt.ip++
			OP := m.code[rt.ip]
			rt.ip++
			index := int(m.code[rt.ip])

			var value Value
			switch OP {
			case op.LOAD_LOCAL:
				value = rt.getLocal(index)
			case op.LOAD_CAPTURED:
				value = rt.getCaptured(index)
			}

			if f, isFloat64 := value.AsFloat64(); isFloat64 {
				value = BoxFloat64(f + 1)
			} else if i, isInt64 := value.AsInt64(); isInt64 {
				value = BoxInt64(i + 1)
			} else {
				return Value{}, CustomError("cannot increment variable '%v' with a value of type '%v'", m.symbolsMap[rt.ip], value.TypeOf())
			}

			switch OP {
			case op.LOAD_LOCAL:
				rt.storeLocal(index, value)
			case op.LOAD_CAPTURED:
				rt.storeCaptured(index, value)
			}
			return value, nil
		},
		func(rt *CoRoutine) (Value, error) { // DEC
			rt.ip++
			OP := m.code[rt.ip]
			rt.ip++
			index := int(m.code[rt.ip])

			var value Value
			switch OP {
			case op.LOAD_LOCAL:
				value = rt.getLocal(index)
			case op.LOAD_CAPTURED:
				value = rt.getCaptured(index)
			}

			if f, isFloat64 := value.AsFloat64(); isFloat64 {
				value = BoxFloat64(f - 1)
			} else if i, isInt64 := value.AsInt64(); isInt64 {
				value = BoxInt64(i - 1)
			} else {
				return Value{}, CustomError("cannot decremenent variable '%v' with a value of type '%v'", m.symbolsMap[rt.ip], value.TypeOf())
			}

			switch OP {
			case op.LOAD_LOCAL:
				rt.storeLocal(index, value)
			case op.LOAD_CAPTURED:
				rt.storeCaptured(index, value)
			}
			return value, nil
		},
		func(rt *CoRoutine) (Value, error) { // STORE_ADD
			rt.ip++
			OP := m.code[rt.ip]
			rt.ip++
			index := int(m.code[rt.ip])

			var left Value
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
					left = BoxInt64(a + b)
					goto SAVE
				} else if b, ok := right.AsFloat64(); ok {
					left = BoxFloat64(float64(a) + b)
					goto SAVE
				}
			} else if a, ok := left.AsFloat64(); ok {
				if b, ok := right.AsInt64(); ok {
					left = BoxFloat64(a + float64(b))
					goto SAVE
				} else if b, ok := right.AsFloat64(); ok {
					left = BoxFloat64(a + b)
					goto SAVE
				}
			}

			return Value{}, CustomError(mathErrFormat, "+", left, right)

		SAVE:
			switch OP {
			case op.LOAD_LOCAL:
				rt.storeLocal(index, left)
			case op.LOAD_CAPTURED:
				rt.storeCaptured(index, left)
			}

			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // STORE_SUB
			rt.ip++
			OP := m.code[rt.ip]
			rt.ip++
			index := int(m.code[rt.ip])

			var left Value
			switch OP {
			case op.LOAD_LOCAL:
				left = *rt.stack[rt.getCurrentBase()+int(index)]
			case op.LOAD_CAPTURED:
				left = *rt.captured[index]
			}

			right, err := rt.next()
			if err != nil {
				return right, err
			}

			if a, ok := left.AsInt64(); ok {
				if b, ok := right.AsInt64(); ok {
					left = BoxInt64(a - b)
					goto SAVE
				} else if b, ok := right.AsFloat64(); ok {
					left = BoxFloat64(float64(a) - b)
					goto SAVE
				}
			} else if a, ok := left.AsFloat64(); ok {
				if b, ok := right.AsInt64(); ok {
					left = BoxFloat64(a - float64(b))
					goto SAVE
				} else if b, ok := right.AsFloat64(); ok {
					left = BoxFloat64(a - b)
					goto SAVE
				}
			}

			return Value{}, CustomError(mathErrFormat, "-", left, right)

		SAVE:
			switch OP {
			case op.LOAD_LOCAL:
				rt.storeLocal(index, left)
			case op.LOAD_CAPTURED:
				rt.storeCaptured(index, left)
			}

			return Value{}, nil
		},
		func(rt *CoRoutine) (Value, error) { // ADD_RIGHT_CONST
			a, err := rt.next()
			if err != nil {
				return a, err
			}

			bOp := m.code[rt.ip+1]
			b := unsafe.Pointer(&m.code[rt.ip+2])
			rt.ip += 9

			if a, ok := a.AsInt64(); ok {
				if bOp == op.INT {
					return BoxInt64(a + *(*int64)(b)), nil
				}
				return BoxFloat64(float64(a) + *(*float64)(b)), nil
			}
			if a, ok := a.AsFloat64(); ok {
				if bOp == op.INT {
					return BoxFloat64(a + float64(*(*int64)(b))), nil
				}
				return BoxFloat64(a + *(*float64)(b)), nil
			}

			return Value{}, CustomError(mathErrFormat, "+", a, b)
		},
		func(rt *CoRoutine) (Value, error) { // SUB_RIGHT_CONST
			a, err := rt.next()
			if err != nil {
				return a, err
			}

			bOp := m.code[rt.ip+1]
			b := unsafe.Pointer(&m.code[rt.ip+2])
			rt.ip += 9

			if a, ok := a.AsInt64(); ok {
				if bOp == op.INT {
					return BoxInt64(a - *(*int64)(b)), nil
				}
				return BoxFloat64(float64(a) - *(*float64)(b)), nil
			}
			if a, ok := a.AsFloat64(); ok {
				if bOp == op.INT {
					return BoxFloat64(a - float64(*(*int64)(b))), nil
				}
				return BoxFloat64(a - *(*float64)(b)), nil
			}

			return Value{}, CustomError(mathErrFormat, "-", a, b)
		},
		func(rt *CoRoutine) (Value, error) { // LS_RIGHT_CONST
			a, err := rt.next()
			if err != nil {
				return a, err
			}

			bOp := m.code[rt.ip+1]
			b := unsafe.Pointer(&m.code[rt.ip+2])
			rt.ip += 9

			if a, ok := a.AsInt64(); ok {
				if bOp == op.INT {
					return BoxBool(a < *(*int64)(b)), nil
				}
				return BoxBool(float64(a) < *(*float64)(b)), nil
			}
			if a, ok := a.AsFloat64(); ok {
				if bOp == op.INT {
					return BoxBool(a < float64(*(*int64)(b))), nil
				}
				return BoxBool(a < *(*float64)(b)), nil
			}

			return Value{}, CustomError(mathErrFormat, "<", a, b)
		},
		func(rt *CoRoutine) (Value, error) { // RETURN_IF
			size := int(m.code[rt.ip+1])
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
			return Value{}, nil
		},
	}
}

func (rt *CoRoutine) Initialize() error {
	AcquireGIL()
	defer ReleaseGIL()

	rt.pushBase(0)
	for rt.ip = 0; rt.ip < len(m.code); rt.ip++ {
		// fetch and execute the instruction
		if _, err := instructions[m.code[rt.ip]](rt); err != nil {
			return err
		}
	}

	return nil
}

// evaluates the next value and returns it
func (rt *CoRoutine) next() (Value, error) {
	rt.ip++
	return instructions[m.code[rt.ip]](rt)
}

// evaluates the next value and returns it, panics on errors
func (rt *CoRoutine) nextP() Value {
	rt.ip++
	v, err := instructions[m.code[rt.ip]](rt)
	if err != nil {
		panic(err)
	}
	return v
}

func (rt *CoRoutine) exitUserFN(oldAddr int, nLocals int, oldEnc []*Value) {
	// return to caller context
	rt.ip = oldAddr
	rt.popLocals(nLocals)
	rt.popBase()
	rt.captured = oldEnc
}

func (fn UserFn) Call(args ...Value) (Value, error) {
	if len(fn.Args) != len(args) {
		if fn.Name != "λ" {
			return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.Name, len(fn.Args), len(args))
		}
		return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.Args), len(args))
	}

	AcquireGIL()
	defer ReleaseGIL()

	// Routines could later be pooled for better performance
	rt := &CoRoutine{stack: make([]*Value, fn.Capacity), basis: []int{0}, captured: fn.captured}
	for i := range fn.Capacity {
		rt.stack[i] = m.boxes.new()
	}

	// run fn
	for rt.ip = fn.Start; rt.ip < fn.End; rt.ip++ {
		if v, err := instructions[m.code[rt.ip]](rt); err != nil {
			if err == errReturnSignal {
				return v, nil
			}
			return v, errWithTrace{err, m.trace}
		}
	}

	// release non-escaping locals
	for _, index := range fn.NonEscaping {
		m.boxes.put(rt.stack[index])
	}

	return Value{}, nil
}

func (rt *CoRoutine) tryNativeCall(value Value, nargsP int) (result Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = Value{}
			err = r.(error)
		}
	}()

	nfn, ok := value.AsNativeFn()
	if !ok {
		return Value{}, errNotFunction
	}

	switch nargsP {
	case 0:
		if fn, ok := nfn.(func() (Value, error)); ok {
			return fn()
		}
	case 1:
		if fn, ok := nfn.(func(Value) (Value, error)); ok {
			return fn(rt.nextP())
		}
	case 2:
		if fn, ok := nfn.(func(Value, Value) (Value, error)); ok {
			return fn(rt.nextP(), rt.nextP())
		}
	case 3:
		if fn, ok := nfn.(func(Value, Value, Value) (Value, error)); ok {
			return fn(rt.nextP(), rt.nextP(), rt.nextP())
		}
	}

	panic("this cant be")
}