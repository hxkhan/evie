package vm

//go:generate go run gen_ops.go

import (
	"fmt"

	"github.com/hxkhan/evie/ast"
)

func (vm *Instance) emitNeg(node ast.Neg) instruction {
	site := vm.at(node)
	if val, ok := vm.evaluate(node).(Value); ok {
		return func(fbr *Fiber) (Value, Exception) {
			return val, nil
		}
	}

	value := vm.compile(node.Value)

	return func(fbr *Fiber) (Value, Exception) {
		value, exc := value(fbr)
		if exc != nil {
			return value, exc
		}

		if float, ok := value.AsFloat64(); ok {
			return BoxNumber(-float), nil
		}
		return Value{}, errorAt(site, "cannot negate %s", article(value))
	}
}

func (vm *Instance) emitNot(node ast.Not) instruction {
	if val, ok := vm.evaluate(node).(Value); ok {
		return func(fbr *Fiber) (Value, Exception) {
			return val, nil
		}
	}

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

func (vm *Instance) emitStringTemplate(node ast.StringTemplate) instruction {
	if val, ok := vm.evaluate(node).(Value); ok {
		return func(fbr *Fiber) (Value, Exception) {
			return val, nil
		}
	}

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
}

func (vm *Instance) emitEcho(node ast.Echo) instruction {
	what := vm.compile(node.Value)

	return func(fbr *Fiber) (Value, Exception) {
		v, err := what(fbr)
		if err != nil {
			return v, err
		}

		fmt.Println(v)
		return Value{}, nil
	}
}

func (vm *Instance) emitIsInstanceOf(node ast.IsInstanceOf) instruction {
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
		} else if rhs == BoxBuiltinType(typeNumber) {
			_, isNumber := lhs.AsFloat64()
			return BoxBool(isNumber), nil
		} else if rhs == BoxBuiltinType(typeBool) {
			_, isBool := lhs.AsBool()
			return BoxBool(isBool), nil
		}

		// host types
		if bt, isType := rhs.AsBuiltinType(); isType {
			if _, t, isHost := lhs.AsHost(); isHost {
				return BoxBool(t.typ == bt), nil
			}
		}

		return BoxBool(false), nil
	}
}
