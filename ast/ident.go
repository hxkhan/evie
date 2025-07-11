package ast

import (
	"github.com/hk-32/evie/core"
)

type IdentDec struct {
	Name  string
	Value Node
}

type IdentGet struct {
	Name string
}

type IdentSet struct {
	Name  string
	Value Node
}

func (iDec IdentDec) compile(cs *Machine) core.Instruction {
	index := cs.declare(iDec.Name)
	value := iDec.Value.compile(cs)

	return func(rt *core.CoRoutine) (core.Value, error) {
		v, err := value(rt)
		if err != nil {
			return v, err
		}

		rt.StoreLocal(index, v)
		return core.Value{}, nil
	}
}

func (iDec IdentDec) compileInGlobal(cs *Machine) core.Instruction {
	index := cs.get(iDec.Name)
	value := iDec.Value.compile(cs)

	return func(rt *core.CoRoutine) (core.Value, error) {
		v, err := value(rt)
		if err != nil {
			return v, err
		}

		rt.StoreLocal(index, v)
		return core.Value{}, nil
	}
}

func (iGet IdentGet) compile(cs *Machine) core.Instruction {
	ref := cs.reach(iGet.Name)

	switch {
	case ref.Scroll < 0:
		return func(rt *core.CoRoutine) (core.Value, error) {
			return cs.Builtins[ref.Index], nil
		}

	case ref.Scroll == 0:
		return func(rt *core.CoRoutine) (core.Value, error) {
			return rt.GetLocal(ref.Index), nil
		}

	case ref.Scroll > 0:
		index := cs.addToCaptured(ref)
		return func(rt *core.CoRoutine) (core.Value, error) {
			return rt.GetCaptured(index), nil
		}
	}

	panic("impossible")
}

func (iSet IdentSet) compile(cs *Machine) core.Instruction {
	ref := cs.reach(iSet.Name)

	value := iSet.Value.compile(cs)
	switch {
	case ref.Scroll < 0:
		panic("cannot set the value of a built-in")

	case ref.Scroll == 0:
		return func(rt *core.CoRoutine) (core.Value, error) {
			v, err := value(rt)
			if err != nil {
				return v, err
			}

			rt.StoreLocal(ref.Index, v)
			return core.Value{}, nil
		}

	case ref.Scroll > 0:
		index := cs.addToCaptured(ref)
		return func(rt *core.CoRoutine) (core.Value, error) {
			v, err := value(rt)
			if err != nil {
				return v, err
			}

			rt.StoreCaptured(index, v)
			return core.Value{}, nil
		}
	}

	panic("impossible")
}

/*
// optimise: n = n+x or n = n-x or n = n*x or n = n/x
if bop, isBinOp := iSet.Value.(BinOp); isBinOp {
	if bop.isOpOneOf(op.ADD, op.SUB, op.MUL) {
		iGet, isIdentGet := bop.A.(IdentGet)
		in, isInput := bop.B.(Input)

		// if left is not IdentGet then try flipping if operation is commutative
		if !isIdentGet && (bop.OP == op.ADD || bop.OP == op.MUL) {
			iGet, isIdentGet = bop.B.(IdentGet)
			in, isInput = bop.A.(Input)
		}

		if isIdentGet && iGet.Name == iSet.Name {
			// n++ or n--
			if isInput {
				switch b := in.Value.(type) {
				case int64:
					switch bop.OP {
					case op.ADD:
						if ref.Scroll == 0 {
							return func(rt *core.CoRoutine) (result core.Value, err error) {
								a := rt.GetLocal(index)

								if i64, ok := a.AsInt64(); ok {
									result = core.BoxInt64(i64 + b)
								} else if f64, ok := a.AsFloat64(); ok {
									result = core.BoxFloat64(f64 + float64(b))
								} else {
									err = core.OperatorTypesError("+", a, b)
								}
								if err != nil {
									return result, err
								}

								rt.StoreLocal(index, result)
								return core.Value{}, nil
							}
						} else {
							return func(rt *core.CoRoutine) (result core.Value, err error) {
								a := rt.GetCaptured(index)

								if i64, ok := a.AsInt64(); ok {
									result = core.BoxInt64(i64 + b)
								} else if f64, ok := a.AsFloat64(); ok {
									result = core.BoxFloat64(f64 + float64(b))
								} else {
									err = core.OperatorTypesError("+", a, b)
								}
								if err != nil {
									return result, err
								}

								rt.StoreCaptured(index, result)
								return core.Value{}, nil
							}
						}
					}
				}
			}

			// compile dynamic x aswell
		}
	}
} */
