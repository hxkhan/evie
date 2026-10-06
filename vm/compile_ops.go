package vm

//go:generate go run gen_ops.go

import (
	"fmt"

	"github.com/hxkhan/evie/ast"
)

// assignee resolves the variable a compound assignment such as `x += 1`
// updates; fields and subscripts are handled before it is called. It looks
// the name up without folding a constant to its value, so a constant is
// reported as one, and it works with the compiler's fast paths off.
func (vm *Instance) assignee(node ast.Node) binding {
	ident, ok := node.(ast.Ident)
	if !ok {
		panic(vm.compileErrorAt(node, "cannot assign to '%v'", node))
	}
	variable, err := vm.cp.reach(ident.Name)
	if err != nil {
		panic(vm.locate(err, ident))
	}
	return variable
}

func (vm *Instance) emitNeg(node ast.Neg) instruction {
	site := vm.at(node)
	if val, ok := vm.evaluate(node).(Value); ok {
		return func(fbr *fiber) (Value, exception) {
			return val, nil
		}
	}

	value := vm.compile(node.Value)

	return func(fbr *fiber) (Value, exception) {
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
		return func(fbr *fiber) (Value, exception) {
			return val, nil
		}
	}

	value := vm.compile(node.Value)

	return func(fbr *fiber) (Value, exception) {
		value, exc := value(fbr)
		if exc != nil {
			return value, exc
		}

		return BoxBool(!value.IsTruthy()), nil
	}
}

func (vm *Instance) emitCatch(node ast.Catch) instruction {
	action := vm.compile(node.Action)

	return func(fbr *fiber) (Value, exception) {
		v, err := action(fbr)
		if err != nil {
			if err != signalReturn && err != signalContinue && err != signalBreak && !err.fatal {
				// convert exception to error-as-value and return
				return boxError(err), nil
			}

			// propagate as normal
			return v, err
		}

		return v, nil
	}
}

func (vm *Instance) emitStringTemplate(node ast.StringTemplate) instruction {
	if val, ok := vm.evaluate(node).(Value); ok {
		return func(fbr *fiber) (Value, exception) {
			return val, nil
		}
	}

	args := make([]instruction, len(node.Args))
	for i, arg := range node.Args {
		args[i] = vm.compile(arg)
	}

	return func(fbr *fiber) (Value, exception) {
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

	return func(fbr *fiber) (Value, exception) {
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

	return func(fbr *fiber) (Value, exception) {
		lhs, exc := lhs(fbr)
		if exc != nil {
			return lhs, exc
		}

		rhs, exc := rhs(fbr)
		if exc != nil {
			return rhs, exc
		}

		// structs
		if us, isUserStruct := rhs.asUserStruct(); isUserStruct {
			if usi, isUserStructInstance := lhs.asStructInstance(); isUserStructInstance {
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
		if rhs == boxBuiltinType(typeString) {
			_, isString := lhs.AsString()
			return BoxBool(isString), nil
		} else if rhs == boxBuiltinType(typeError) {
			_, isError := lhs.AsError()
			return BoxBool(isError), nil
		} else if rhs == boxBuiltinType(typeArray) {
			_, isArray := lhs.AsArray()
			return BoxBool(isArray), nil
		} else if rhs == boxBuiltinType(typeObject) {
			_, isObj := lhs.AsObject()
			return BoxBool(isObj), nil
		} else if rhs == boxBuiltinType(typeBuffer) {
			_, isBuf := lhs.AsBuffer()
			return BoxBool(isBuf), nil
		} else if rhs == boxBuiltinType(typeTask) {
			_, isTask := lhs.AsTask()
			return BoxBool(isTask), nil
		} else if rhs == boxBuiltinType(typeNumber) {
			_, isNumber := lhs.AsFloat64()
			return BoxBool(isNumber), nil
		} else if rhs == boxBuiltinType(typeBool) {
			_, isBool := lhs.AsBool()
			return BoxBool(isBool), nil
		}

		// host types
		if bt, isType := rhs.asBuiltinType(); isType {
			if _, t, isHost := lhs.AsHost(); isHost {
				return BoxBool(t.typ == bt), nil
			}
		}

		return BoxBool(false), nil
	}
}
