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

func (bop BinOp) compile(cs *Machine) core.Instruction {
	lhs := bop.A.compile(cs)
	rhs := bop.B.compile(cs)

	// optimise: x + constant
	if in, isInput := bop.B.(Input); isInput && cs.optimise {
		switch b := in.Value.(type) {
		case int64:
			switch bop.OP {
			case op.ADD:
				return func(rt *core.CoRoutine) (core.Value, error) {
					a, err := lhs(rt)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsInt64(); ok {
						return core.BoxInt64(a + b), nil
					}

					if a, ok := a.AsFloat64(); ok {
						return core.BoxFloat64(a + float64(b)), nil
					}

					return core.Value{}, core.OperatorTypesError("+", a, b)
				}

			case op.SUB:
				return func(rt *core.CoRoutine) (core.Value, error) {
					a, err := lhs(rt)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsInt64(); ok {
						return core.BoxInt64(a - b), nil
					}

					if a, ok := a.AsFloat64(); ok {
						return core.BoxFloat64(a - float64(b)), nil
					}

					return core.Value{}, core.OperatorTypesError("-", a, b)
				}

			case op.LS:
				return func(rt *core.CoRoutine) (core.Value, error) {
					a, err := lhs(rt)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsInt64(); ok {
						return core.BoxBool(a < b), nil
					}

					if a, ok := a.AsFloat64(); ok {
						return core.BoxBool(a < float64(b)), nil
					}

					return core.Value{}, core.OperatorTypesError("<", a, b)
				}
			case op.MR:
				return func(rt *core.CoRoutine) (core.Value, error) {
					a, err := lhs(rt)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsInt64(); ok {
						return core.BoxBool(a > b), nil
					}

					if a, ok := a.AsFloat64(); ok {
						return core.BoxBool(a > float64(b)), nil
					}

					return core.Value{}, core.OperatorTypesError(">", a, b)
				}
			}
		}
	}

	// generic compilation
	switch bop.OP {
	case op.ADD:
		return func(rt *core.CoRoutine) (core.Value, error) {
			a, err := lhs(rt)
			if err != nil {
				return a, err
			}
			b, err := rhs(rt)
			if err != nil {
				return a, err
			}
			if a, ok := a.AsInt64(); ok {
				if b, ok := b.AsInt64(); ok {
					return core.BoxInt64(a + b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return core.BoxFloat64(float64(a) + b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return core.BoxFloat64(a + float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return core.BoxFloat64(a + b), nil
				}
			}

			return core.Value{}, core.OperatorTypesError("+", a, b)
		}

	case op.SUB:
		return func(rt *core.CoRoutine) (core.Value, error) {
			a, err := lhs(rt)
			if err != nil {
				return a, err
			}
			b, err := rhs(rt)
			if err != nil {
				return a, err
			}
			if a, ok := a.AsInt64(); ok {
				if b, ok := b.AsInt64(); ok {
					return core.BoxInt64(a - b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return core.BoxFloat64(float64(a) - b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return core.BoxFloat64(a - float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return core.BoxFloat64(a - b), nil
				}
			}

			return core.Value{}, core.OperatorTypesError("-", a, b)
		}

	case op.LS:
		return func(rt *core.CoRoutine) (core.Value, error) {
			a, err := lhs(rt)
			if err != nil {
				return a, err
			}
			b, err := rhs(rt)
			if err != nil {
				return a, err
			}
			if a, ok := a.AsInt64(); ok {
				if b, ok := b.AsInt64(); ok {
					return core.BoxBool(a < b), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return core.BoxBool(float64(a) < b), nil
				}
			}

			if a, ok := a.AsFloat64(); ok {
				if b, ok := b.AsInt64(); ok {
					return core.BoxBool(a < float64(b)), nil
				}
				if b, ok := b.AsFloat64(); ok {
					return core.BoxBool(a < b), nil
				}
			}

			return core.Value{}, core.OperatorTypesError("<", a, b)
		}
	}
	panic("OP not implemented")
}

type Neg struct {
	O Node // [required]
}

func (neg Neg) compile(cs *Machine) core.Instruction {
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

func adder(lhs core.Value, rhs any) (core.Value, error) {
	if a, ok := lhs.AsInt64(); ok {
		if b, ok := rhs.(int64); ok {
			return core.BoxInt64(a + b), nil
		}
		if b, ok := rhs.(float64); ok {
			return core.BoxFloat64(float64(a) + b), nil
		}
	}

	if a, ok := lhs.AsFloat64(); ok {
		if b, ok := rhs.(int64); ok {
			return core.BoxFloat64(a + float64(b)), nil
		}
		if b, ok := rhs.(float64); ok {
			return core.BoxFloat64(a + b), nil
		}
	}

	return core.Value{}, core.OperatorTypesError("+", lhs, rhs)
}

/* if in, isInput := bop.B.(Input); isInput && cs.optimise {
	switch b := in.Value.(type) {
	case int64:
		switch bop.OP {
		case op.ADD:
			return func(rt *core.CoRoutine) (core.Value, error) {
				a, err := lhs(rt)
				if err != nil {
					return a, err
				}

				if a, ok := a.AsInt64(); ok {
					return core.BoxInt64(a + b), nil
				}

				if a, ok := a.AsFloat64(); ok {
					return core.BoxFloat64(a + float64(b)), nil
				}

				return core.Value{}, core.OperatorTypesError("+", a, b)
			}

		case op.SUB:
			return func(rt *core.CoRoutine) (core.Value, error) {
				a, err := lhs(rt)
				if err != nil {
					return a, err
				}

				if a, ok := a.AsInt64(); ok {
					return core.BoxInt64(a - b), nil
				}

				if a, ok := a.AsFloat64(); ok {
					return core.BoxFloat64(a - float64(b)), nil
				}

				return core.Value{}, core.OperatorTypesError("-", a, b)
			}

		case op.LS:
			return func(rt *core.CoRoutine) (core.Value, error) {
				a, err := lhs(rt)
				if err != nil {
					return a, err
				}

				if a, ok := a.AsInt64(); ok {
					return core.BoxBool(a < b), nil
				}

				if a, ok := a.AsFloat64(); ok {
					return core.BoxBool(a < float64(b)), nil
				}

				return core.Value{}, core.OperatorTypesError("<", a, b)
			}
		}
	}
} */
