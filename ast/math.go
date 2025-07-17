package ast

import (
	"slices"

	"github.com/hk-32/evie/core"
)

type Operator int

const (
	AddOp Operator = iota + 1
	SubOp
	MulOp
	DivOp

	EqOp
	LtOp
	GtOp
)

type BinOp struct {
	Lhs      Node // [required]
	Operator      // [required]
	Rhs      Node // [required]
}

func (bop BinOp) isOpOneOf(ops ...Operator) bool {
	return slices.Contains(ops, bop.Operator)
}

func (bop BinOp) compile(vm *Machine) core.Instruction {
	// optimise: lhs being a local variable
	if iGet, isIdentGet := bop.Lhs.(IdentGet); isIdentGet && vm.optimise {
		ref, err := vm.reach(iGet.Name)
		if err != nil {
			panic(err)
		}

		if ref.IsLocal() {
			// optimise: rhs being a constant
			if in, isInput := bop.Rhs.(Input); isInput {
				if b, isFloat := in.Value.AsFloat64(); isFloat {
					switch bop.Operator {
					case AddOp:
						return func(fbr *core.Fiber) (core.Value, error) {
							a := fbr.GetLocal(ref.Index)
							if a, ok := a.AsFloat64(); ok {
								return core.BoxFloat64(a + b), nil
							}
							return core.Value{}, core.OperatorTypesError("+", a, b)
						}

					case SubOp:
						return func(fbr *core.Fiber) (core.Value, error) {
							a := fbr.GetLocal(ref.Index)
							if a, ok := a.AsFloat64(); ok {
								return core.BoxFloat64(a - b), nil
							}
							return core.Value{}, core.OperatorTypesError("-", a, b)
						}

					case LtOp:
						return func(fbr *core.Fiber) (core.Value, error) {
							a := fbr.GetLocal(ref.Index)
							if a, ok := a.AsFloat64(); ok {
								return core.BoxBool(a < b), nil
							}
							return core.Value{}, core.OperatorTypesError("<", a, b)
						}
					}
				}
			}

			// generic rhs
			rhs := bop.Rhs.compile(vm)
			switch bop.Operator {
			case AddOp:
				return func(fbr *core.Fiber) (core.Value, error) {
					a := fbr.GetLocal(ref.Index)
					b, err := rhs(fbr)
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
			case SubOp:
				return func(fbr *core.Fiber) (core.Value, error) {
					a := fbr.GetLocal(ref.Index)
					b, err := rhs(fbr)
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

			case LtOp:
				return func(fbr *core.Fiber) (core.Value, error) {
					a := fbr.GetLocal(ref.Index)
					b, err := rhs(fbr)
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

	lhs := bop.Lhs.compile(vm)
	// optimise: rhs being a constant
	if in, isInput := bop.Rhs.(Input); isInput && vm.optimise {
		if b, isFloat := in.Value.AsFloat64(); isFloat {
			switch bop.Operator {
			case AddOp:
				return func(fbr *core.Fiber) (core.Value, error) {
					a, err := lhs(fbr)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsFloat64(); ok {
						return core.BoxFloat64(a + float64(b)), nil
					}
					return core.Value{}, core.OperatorTypesError("+", a, b)
				}

			case SubOp:
				return func(fbr *core.Fiber) (core.Value, error) {
					a, err := lhs(fbr)
					if err != nil {
						return a, err
					}

					if a, ok := a.AsFloat64(); ok {
						return core.BoxFloat64(a - float64(b)), nil
					}
					return core.Value{}, core.OperatorTypesError("-", a, b)
				}

			case LtOp:
				return func(fbr *core.Fiber) (core.Value, error) {
					a, err := lhs(fbr)
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

	rhs := bop.Rhs.compile(vm)

	// generic compilation
	switch bop.Operator {
	case AddOp:
		return func(fbr *core.Fiber) (core.Value, error) {
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
					return core.BoxFloat64(a + b), nil
				}
			}

			return core.Value{}, core.OperatorTypesError("+", a, b)
		}

	case SubOp:
		return func(fbr *core.Fiber) (core.Value, error) {
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
					return core.BoxFloat64(a - b), nil
				}
			}

			return core.Value{}, core.OperatorTypesError("-", a, b)
		}

	case LtOp:
		return func(fbr *core.Fiber) (core.Value, error) {
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
