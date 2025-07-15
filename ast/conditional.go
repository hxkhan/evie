package ast

import (
	"github.com/hk-32/evie/core"
)

type Conditional struct {
	Condition Node // [required]
	Action    Node // [required]
	Otherwise Node // [optional]
}

func (cond Conditional) compile(cs *Machine) core.Instruction {
	// optimise: if (something) return x
	if cond.Otherwise == nil && cs.optimise {
		if ret, isReturn := cond.Action.(Return); isReturn {
			condition := cond.Condition.compile(cs)
			// optimise: returning contants
			if in, isInput := ret.Value.(Input); isInput {
				return func(rt *core.CoRoutine) (core.Value, error) {
					v, err := condition(rt)
					if err != nil {
						return v, err
					}

					if v.IsTruthy() {
						return in.Value, core.ErrReturnSignal
					}
					return core.Value{}, nil
				}
			}

			// optimise: returning local variables
			if iGet, isIdentGet := ret.Value.(IdentGet); isIdentGet && cs.optimise {
				ref := cs.reach(iGet.Name)
				if ref.IsLocal() {
					return func(rt *core.CoRoutine) (core.Value, error) {
						v, err := condition(rt)
						if err != nil {
							return v, err
						}

						if v.IsTruthy() {
							return rt.GetLocal(ref.Index), core.ErrReturnSignal
						}
						return core.Value{}, nil
					}
				}
			}

			// generic compilation
			what := ret.Value.compile(cs)
			return func(rt *core.CoRoutine) (core.Value, error) {
				v, err := condition(rt)
				if err != nil {
					return v, err
				}

				if v.IsTruthy() {
					v, err := what(rt)
					if err != nil {
						return v, err
					}
					return v, core.ErrReturnSignal
				}
				return core.Value{}, nil
			}
		}
	}

	// generic compilation
	condition := cond.Condition.compile(cs)

	cs.scopeOpenBlock()
	action := cond.Action.compile(cs)

	if cond.Otherwise != nil {
		var otherwise core.Instruction
		if o, isELIF := cond.Otherwise.(Conditional); isELIF {
			otherwise = o.compileAsELIF(cs)
		} else {
			// means it's an else
			cs.scopeReuseBlock()
			otherwise = cond.Otherwise.compile(cs)
		}

		cs.scopeCloseBlock()
		return func(rt *core.CoRoutine) (core.Value, error) {
			v, err := condition(rt)
			if err != nil {
				return v, err
			}

			if v.IsTruthy() {
				return action(rt)
			}
			return otherwise(rt)
		}
	}
	cs.scopeCloseBlock()

	return func(rt *core.CoRoutine) (core.Value, error) {
		v, err := condition(rt)
		if err != nil {
			return v, err
		}

		if v.IsTruthy() {
			return action(rt)
		}
		return core.Value{}, nil
	}
}

func (cond Conditional) compileAsELIF(cs *Machine) core.Instruction {
	condition := cond.Condition.compile(cs)

	cs.scopeReuseBlock()
	action := cond.Action.compile(cs)

	if cond.Otherwise != nil {
		var otherwise core.Instruction
		if o, isELIF := cond.Otherwise.(Conditional); isELIF {
			otherwise = o.compileAsELIF(cs)
		} else {
			// means it's an else
			cs.scopeReuseBlock()
			otherwise = cond.Otherwise.compile(cs)
		}

		return func(rt *core.CoRoutine) (core.Value, error) {
			v, err := condition(rt)
			if err != nil {
				return v, err
			}

			if v.IsTruthy() {
				return action(rt)
			}
			return otherwise(rt)
		}
	}

	return func(rt *core.CoRoutine) (core.Value, error) {
		v, err := condition(rt)
		if err != nil {
			return v, err
		}

		if v.IsTruthy() {
			return action(rt)
		}
		return core.Value{}, nil
	}
}
