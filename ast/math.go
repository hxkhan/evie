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

func (bop BinOp) compile(vm *Machine) core.Instruction {
	// optimise: lhs being a local variable
	if iGet, isIdentGet := bop.A.(IdentGet); isIdentGet && vm.optimise {
		ref, err := vm.reach(iGet.Name)
		if err != nil {
			panic(err)
		}

		if ref.IsLocal() {
			// optimise: rhs being a constant
			if in, isInput := bop.B.(Input); isInput {
				if b, isFloat := in.Value.AsFloat64(); isFloat {
					switch bop.OP {
					case op.ADD:
						return func(rt *core.CoRoutine) (core.Value, error) {
							a := rt.GetLocal(ref.Index)
							if a, ok := a.AsFloat64(); ok {
								return core.BoxFloat64(a + b), nil
							}
							return core.Value{}, core.OperatorTypesError("+", a, b)
						}

					case op.SUB:
						return func(rt *core.CoRoutine) (core.Value, error) {
							a := rt.GetLocal(ref.Index)
							if a, ok := a.AsFloat64(); ok {
								return core.BoxFloat64(a - b), nil
							}
							return core.Value{}, core.OperatorTypesError("-", a, b)
						}

					case op.LS:
						return func(rt *core.CoRoutine) (core.Value, error) {
							a := rt.GetLocal(ref.Index)
							if a, ok := a.AsFloat64(); ok {
								return core.BoxBool(a < b), nil
							}
							return core.Value{}, core.OperatorTypesError("<", a, b)
						}
					}
				}
			}

			// generic rhs
			rhs := bop.B.compile(vm)
			switch bop.OP {
			case op.ADD:
				return func(rt *core.CoRoutine) (core.Value, error) {
					a := rt.GetLocal(ref.Index)
					b, err := rhs(rt)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsFloat64(); ok {
						if b, ok := b.AsFloat64(); ok {
							return core.BoxFloat64(a + b), nil
						}
					}
					return core.Value{}, core.OperatorTypesError("+", a, b)
				}
			case op.SUB:
				return func(rt *core.CoRoutine) (core.Value, error) {
					a := rt.GetLocal(ref.Index)
					b, err := rhs(rt)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsFloat64(); ok {
						if b, ok := b.AsFloat64(); ok {
							return core.BoxFloat64(a - b), nil
						}
					}
					return core.Value{}, core.OperatorTypesError("-", a, b)
				}

			case op.LS:
				return func(rt *core.CoRoutine) (core.Value, error) {
					a := rt.GetLocal(ref.Index)
					b, err := rhs(rt)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsFloat64(); ok {
						if b, ok := b.AsFloat64(); ok {
							return core.BoxBool(a < b), nil
						}
					}
					return core.Value{}, core.OperatorTypesError("<", a, b)
				}
			}
		}
	}

	lhs := bop.A.compile(vm)
	// optimise: rhs being a constant
	if in, isInput := bop.B.(Input); isInput && vm.optimise {
		if b, isFloat := in.Value.AsFloat64(); isFloat {
			switch bop.OP {
			case op.ADD:
				return func(rt *core.CoRoutine) (core.Value, error) {
					a, err := lhs(rt)
					if err != nil {
						return a, err
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

					if a, ok := a.AsFloat64(); ok {
						return core.BoxBool(a < float64(b)), nil
					}
					return core.Value{}, core.OperatorTypesError("<", a, b)
				}
			}
		}
	}

	rhs := bop.B.compile(vm)

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

			if a, ok := a.AsFloat64(); ok {
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

			if a, ok := a.AsFloat64(); ok {
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

			if a, ok := a.AsFloat64(); ok {
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

func (neg Neg) compile(vm *Machine) core.Instruction {
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
		neg.O.compile(vm)
	}

	return pos */

	panic("implement")
}
