package ast

import (
	"slices"

	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/op"
)

type BinOp struct {
	OP byte // [required]
	A  Node // [required]
	B  Node // [required]
}

func (bop BinOp) isOpOneOf(ops ...byte) bool {
	return slices.Contains(ops, bop.OP)
}

func (bop BinOp) compile(cs *Machine) {
	bop.A.compile(cs)
	bop.B.compile(cs)

	switch bop.OP {
	case op.ADD:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rhs := rt.Stack[len(rt.Stack)-1]
			lhs := rt.Stack[len(rt.Stack)-2]
			rt.Stack = rt.Stack[:len(rt.Stack)-2]

			result, err := func() (core.Value, error) {
				if a, ok := lhs.AsInt64(); ok {
					if b, ok := rhs.AsInt64(); ok {
						return core.BoxInt64(a + b), nil
					}
					if b, ok := rhs.AsFloat64(); ok {
						return core.BoxFloat64(float64(a) + b), nil
					}
				}

				if a, ok := lhs.AsFloat64(); ok {
					if b, ok := rhs.AsInt64(); ok {
						return core.BoxFloat64(a + float64(b)), nil
					}
					if b, ok := rhs.AsFloat64(); ok {
						return core.BoxFloat64(a + b), nil
					}
				}

				return core.Value{}, core.OperatorTypesError("+", lhs, rhs)
			}()

			rt.Stack = append(rt.Stack, result)
			return 1, err
		})

	case op.SUB:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rhs := rt.Stack[len(rt.Stack)-1]
			lhs := rt.Stack[len(rt.Stack)-2]
			rt.Stack = rt.Stack[:len(rt.Stack)-2]

			result, err := func() (core.Value, error) {
				if a, ok := lhs.AsInt64(); ok {
					if b, ok := rhs.AsInt64(); ok {
						return core.BoxInt64(a - b), nil
					}
					if b, ok := rhs.AsFloat64(); ok {
						return core.BoxFloat64(float64(a) - b), nil
					}
				}

				if a, ok := lhs.AsFloat64(); ok {
					if b, ok := rhs.AsInt64(); ok {
						return core.BoxFloat64(a - float64(b)), nil
					}
					if b, ok := rhs.AsFloat64(); ok {
						return core.BoxFloat64(a - b), nil
					}
				}

				return core.Value{}, core.OperatorTypesError("-", lhs, rhs)
			}()

			rt.Stack = append(rt.Stack, result)
			return 1, err
		})

	case op.LS:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rhs := rt.Stack[len(rt.Stack)-1]
			lhs := rt.Stack[len(rt.Stack)-2]
			rt.Stack = rt.Stack[:len(rt.Stack)-2]

			result, err := func() (core.Value, error) {
				if a, ok := lhs.AsInt64(); ok {
					if b, ok := rhs.AsInt64(); ok {
						return core.BoxBool(a < b), nil
					}
					if b, ok := rhs.AsFloat64(); ok {
						return core.BoxBool(float64(a) < b), nil
					}
				}

				if a, ok := lhs.AsFloat64(); ok {
					if b, ok := rhs.AsInt64(); ok {
						return core.BoxBool(a < float64(b)), nil
					}
					if b, ok := rhs.AsFloat64(); ok {
						return core.BoxBool(a < b), nil
					}
				}

				return core.Value{}, core.OperatorTypesError("<", lhs, rhs)
			}()

			rt.Stack = append(rt.Stack, result)
			return 1, err
		})

	case op.MR:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rhs := rt.Stack[len(rt.Stack)-1]
			lhs := rt.Stack[len(rt.Stack)-2]
			rt.Stack = rt.Stack[:len(rt.Stack)-2]

			result, err := func() (core.Value, error) {
				if a, ok := lhs.AsInt64(); ok {
					if b, ok := rhs.AsInt64(); ok {
						return core.BoxBool(a > b), nil
					}
					if b, ok := rhs.AsFloat64(); ok {
						return core.BoxBool(float64(a) > b), nil
					}
				}

				if a, ok := lhs.AsFloat64(); ok {
					if b, ok := rhs.AsInt64(); ok {
						return core.BoxBool(a > float64(b)), nil
					}
					if b, ok := rhs.AsFloat64(); ok {
						return core.BoxBool(a > b), nil
					}
				}

				return core.Value{}, core.OperatorTypesError(">", lhs, rhs)
			}()

			rt.Stack = append(rt.Stack, result)
			return 1, err
		})
	default:
		panic("OP not implemented")
	}
}

type Neg struct {
	O Node // [required]
}

func (neg Neg) compile(cs *Machine) {
	/* if in, isInput := neg.O.(Input); isInput {
		switch v := in.Value.(type) {
		case int64:
			pos = cs.emitInt64(-v)
		case float64:
			pos = cs.emitFloat64(-v)
		default:
			panic("negation on unsuported data type")
		}
	} else {
		pos = cs.emit(op.NEG)
		neg.O.compile(cs)
	}

	return pos */

	panic("implement")
}
