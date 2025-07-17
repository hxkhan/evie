package ast

import (
	"github.com/hk-32/evie/core"
)

type Conditional struct {
	Condition Node // [required]
	Action    Node // [required]
	Otherwise Node // [optional]
}

func (cond Conditional) compile(vm *Machine) core.Instruction {
	// optimise: if (something) return x
	if cond.Otherwise == nil && vm.optimise {
		if ret, isReturn := cond.Action.(Return); isReturn {
			condition := cond.Condition.compile(vm)
			// optimise: returning constants
			if in, isInput := ret.Value.(Input); isInput {
				return func(fbr *core.Fiber) (core.Value, error) {
					v, err := condition(fbr)
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
			if iGet, isIdentGet := ret.Value.(IdentGet); isIdentGet {
				ref, err := vm.reach(iGet.Name)
				if err != nil {
					panic(err)
				}

				if ref.IsLocal() {
					return func(fbr *core.Fiber) (core.Value, error) {
						v, err := condition(fbr)
						if err != nil {
							return v, err
						}

						if v.IsTruthy() {
							return fbr.GetLocal(ref.Index), core.ErrReturnSignal
						}
						return core.Value{}, nil
					}
				}
			}

			// generic compilation
			what := ret.Value.compile(vm)
			return func(fbr *core.Fiber) (core.Value, error) {
				v, err := condition(fbr)
				if err != nil {
					return v, err
				}

				if v.IsTruthy() {
					v, err := what(fbr)
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
	condition := cond.Condition.compile(vm)

	vm.scope.OpenBlock()
	action := cond.Action.compile(vm)

	if cond.Otherwise != nil {
		var otherwise core.Instruction
		if o, isELIF := cond.Otherwise.(Conditional); isELIF {
			otherwise = o.compileAsELIF(vm)
		} else {
			// it is the else block
			vm.scope.ReuseBlock()
			otherwise = cond.Otherwise.compile(vm)
		}

		vm.scope.CloseBlock()
		return func(fbr *core.Fiber) (core.Value, error) {
			v, err := condition(fbr)
			if err != nil {
				return v, err
			}

			if v.IsTruthy() {
				return action(fbr)
			}
			return otherwise(fbr)
		}
	}

	vm.scope.CloseBlock()
	return func(fbr *core.Fiber) (core.Value, error) {
		v, err := condition(fbr)
		if err != nil {
			return v, err
		}

		if v.IsTruthy() {
			return action(fbr)
		}
		return core.Value{}, nil
	}
}

func (cond Conditional) compileAsELIF(vm *Machine) core.Instruction {
	condition := cond.Condition.compile(vm)

	vm.scope.ReuseBlock()
	action := cond.Action.compile(vm)

	if cond.Otherwise != nil {
		var otherwise core.Instruction
		if o, isELIF := cond.Otherwise.(Conditional); isELIF {
			otherwise = o.compileAsELIF(vm)
		} else {
			// it is the else block
			vm.scope.ReuseBlock()
			otherwise = cond.Otherwise.compile(vm)
		}

		return func(fbr *core.Fiber) (core.Value, error) {
			v, err := condition(fbr)
			if err != nil {
				return v, err
			}

			if v.IsTruthy() {
				return action(fbr)
			}
			return otherwise(fbr)
		}
	}

	return func(fbr *core.Fiber) (core.Value, error) {
		v, err := condition(fbr)
		if err != nil {
			return v, err
		}

		if v.IsTruthy() {
			return action(fbr)
		}
		return core.Value{}, nil
	}
}
