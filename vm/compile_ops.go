package vm

//go:generate go run gen_ops.go

import (
	"fmt"
	"math"
	"strconv"
	"strings"

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

func (vm *Instance) emitPlus(node ast.Plus) instruction {
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

		if _, ok := value.AsFloat64(); ok {
			return value, nil
		}
		return Value{}, errorAt(site, "cannot apply unary '+' to %s", article(value))
	}
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

	parts := node.Parts
	size := 0
	for _, p := range parts {
		size += len(p)
	}

	return func(fbr *fiber) (Value, exception) {
		var b strings.Builder
		b.Grow(size + 8*len(args))
		b.WriteString(parts[0])
		for i, arg := range args {
			v, err := arg(fbr)
			if err != nil {
				return v, err
			}
			writeInterpolated(&b, v)
			b.WriteString(parts[i+1])
		}
		return BoxString(b.String()), nil
	}
}

// writeInterpolated writes a value the way a template shows it: as its String
func writeInterpolated(b *strings.Builder, v Value) {
	switch {
	case v.pointer == f64Type:
		var buf [32]byte
		b.Write(strconv.AppendFloat(buf[:0], math.Float64frombits(v.scalar), 'f', -1, 64))
	case !isKnown(v.pointer) && v.scalar == kindString:
		b.WriteString(*(*string)(v.pointer))
	default:
		b.WriteString(v.String())
	}
}

func (vm *Instance) emitEcho(node ast.Echo) instruction {
	what := vm.compile(node.Value)
	out := vm.rt.stdout // the engine's standard output, not the process's

	return func(fbr *fiber) (Value, exception) {
		v, err := what(fbr)
		if err != nil {
			return v, err
		}

		fmt.Fprintln(out, v)
		return Value{}, nil
	}
}

func (vm *Instance) emitIsInstanceOf(node ast.IsInstanceOf) instruction {
	if rhsVal, ok := vm.evaluate(node.Rhs).(Value); ok {
		if uc, isUserClass := rhsVal.asUserClass(); isUserClass {
			if lhsLocal, ok := vm.evaluate(node.Lhs).(local); ok {
				return func(fbr *fiber) (Value, exception) {
					if ci, isClassInstance := fbr.get(lhsLocal).asClassInstance(); isClassInstance {
						for cur := ci.InstanceOf; cur != nil; cur = cur.Parent {
							if cur == uc {
								return BoxBool(true), nil
							}
						}
					}
					return BoxBool(false), nil
				}
			}
			lhs := vm.compile(node.Lhs)
			return func(fbr *fiber) (Value, exception) {
				lhsVal, exc := lhs(fbr)
				if exc != nil {
					return lhsVal, exc
				}
				if ci, isClassInstance := lhsVal.asClassInstance(); isClassInstance {
					for cur := ci.InstanceOf; cur != nil; cur = cur.Parent {
						if cur == uc {
							return BoxBool(true), nil
						}
					}
				}
				return BoxBool(false), nil
			}
		}
	}

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

		// classes
		if uc, isUserClass := rhs.asUserClass(); isUserClass {
			if ci, isClassInstance := lhs.asClassInstance(); isClassInstance {
				for cur := ci.InstanceOf; cur != nil; cur = cur.Parent {
					if cur == uc {
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
