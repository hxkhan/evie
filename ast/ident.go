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

func (iDec IdentDec) compile(cs *Machine) {
	index := cs.declare(iDec.Name)
	iDec.Value.compile(cs)

	cs.emit(func(rt *core.CoRoutine) (int, error) {
		v := rt.Stack[len(rt.Stack)-1]
		rt.Stack = rt.Stack[:len(rt.Stack)-1]

		rt.StoreLocal(index, v)
		return 1, nil
	})
}

func (iDec IdentDec) compileInGlobal(cs *Machine) {
	index := cs.get(iDec.Name)
	iDec.Value.compile(cs)

	cs.emit(func(rt *core.CoRoutine) (int, error) {
		v := rt.Stack[len(rt.Stack)-1]
		rt.Stack = rt.Stack[:len(rt.Stack)-1]

		rt.StoreLocal(index, v)
		return 1, nil
	})
}

func (iGet IdentGet) compile(cs *Machine) {
	ref := cs.reach(iGet.Name)

	switch {
	case ref.Scroll < 0:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rt.Stack = append(rt.Stack, cs.Builtins[ref.Index])
			return 1, nil
		})

	case ref.Scroll == 0:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rt.Stack = append(rt.Stack, rt.GetLocal(ref.Index))
			return 1, nil
		})

	case ref.Scroll > 0:
		index := cs.addToCaptured(ref)
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			rt.Stack = append(rt.Stack, rt.GetCaptured(index))
			return 1, nil
		})
	}
}

func (iSet IdentSet) compile(cs *Machine) {
	ref := cs.reach(iSet.Name)

	iSet.Value.compile(cs)
	switch {
	case ref.Scroll < 0:
		panic("cannot set the value of a built-in")

	case ref.Scroll == 0:
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			v := rt.Stack[len(rt.Stack)-1]
			rt.Stack = rt.Stack[:len(rt.Stack)-1]

			rt.StoreLocal(ref.Index, v)
			return 1, nil
		})

	case ref.Scroll > 0:
		index := cs.addToCaptured(ref)
		cs.emit(func(rt *core.CoRoutine) (int, error) {
			v := rt.Stack[len(rt.Stack)-1]
			rt.Stack = rt.Stack[:len(rt.Stack)-1]

			rt.StoreCaptured(index, v)
			return 1, nil
		})
	}
}
