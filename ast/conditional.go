package ast

import (
	"github.com/hk-32/evie/core"
)

type Conditional struct {
	Condition Node // [required]
	Action    Node // [required]
	Otherwise Node // [optional]
}

func (cond Conditional) compile(cs *Machine) {
	cond.Condition.compile(cs)

	start := len(cs.Code)
	cs.emit(nil)

	cs.scopeOpenBlock()
	cond.Action.compile(cs)
	skip := len(cs.Code) - start

	if cond.Otherwise != nil {
		panic("fix")
		/* if o, isELIF := cond.Otherwise.(Conditional); isELIF {
			o.compileAsELIF(cs)
		} else {
			// means it's an else
			cs.scopeReuseBlock()
			cond.Otherwise.compile(cs)
		}

		cs.scopeCloseBlock()
		cs.Code[start] = func(rt *core.CoRoutine) (int, error) {
			v := rt.Stack[len(rt.Stack)-1]
			rt.Stack = rt.Stack[:len(rt.Stack)-1]

			if !v.IsTruthy() {
				return skip, nil
			}
			return 1, nil
		}
		return */
	}

	cs.scopeCloseBlock()
	cs.Code[start] = func(rt *core.CoRoutine) (int, error) {
		v := rt.Stack[len(rt.Stack)-1]
		rt.Stack = rt.Stack[:len(rt.Stack)-1]

		if !v.IsTruthy() {
			return skip, nil
		}
		return 1, nil
	}
}

func (cond Conditional) compileAsELIF(cs *Machine) {
	cond.Condition.compile(cs)

	cs.scopeReuseBlock()

	start := len(cs.Code)
	cond.Action.compile(cs)
	actionSize := len(cs.Code) - start

	if cond.Otherwise != nil {
		if o, isELIF := cond.Otherwise.(Conditional); isELIF {
			o.compileAsELIF(cs)
		} else {
			// means it's an else
			cs.scopeReuseBlock()
			cond.Otherwise.compile(cs)
		}

		cs.emit(func(rt *core.CoRoutine) (int, error) {
			v := rt.Stack[len(rt.Stack)-1]
			rt.Stack = rt.Stack[:len(rt.Stack)-1]

			if !v.IsTruthy() {
				return actionSize, nil
			}
			return 1, nil
		})
		return
	}

	cs.emit(func(rt *core.CoRoutine) (int, error) {
		v := rt.Stack[len(rt.Stack)-1]
		rt.Stack = rt.Stack[:len(rt.Stack)-1]

		if !v.IsTruthy() {
			return actionSize, nil
		}
		return 1, nil
	})
}
