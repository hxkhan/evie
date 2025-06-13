package ast

import (
	"github.com/hk-32/evie/op"
)

type Fn struct {
	Name   string
	Args   []string
	Action Node
}

type Go struct {
	Routine Node
}

type Call struct {
	Fn   Node
	Args []Node
}

type Return struct {
	Value Node
}

// LAMBDA : len : name : nargs : (len : arg)... : nrefs : (index, scroll)... : ncap : nrecyclable : (index)... : start : end

func (fn Fn) compile(cs *CompilerState) int {
	if fn.Name != "" {
		panic("named functions are only allowed as top level declarations")
	}

	pos := cs.emit(op.LAMBDA)
	cs.openFunction(pos)

	cs.scopeExtend()
	for _, arg := range fn.Args {
		cs.declare(arg)
	}

	// get/create the info obj if it does not exist
	info := cs.getFnInfoFor(pos)
	fn.Action.compile(cs)

	info.Name = "λ"
	info.Args = fn.Args
	info.Start = pos + 1
	info.End = cs.len()
	info.Capacity = cs.scopeCapacity()

	cs.closeFunction()
	cs.scopeDeExtend()

	cs.emit(op.END)

	return pos
}

func (fn Fn) compileInGlobal(cs *CompilerState) int {
	index := cs.get(fn.Name)
	pos := cs.emit(op.FN_DECL, byte(index))
	cs.openFunction(pos)

	cs.scopeExtend()
	for _, arg := range fn.Args {
		cs.declare(arg)
	}

	// get/create the info obj if it does not exist
	info := cs.getFnInfoFor(pos)
	fn.Action.compile(cs)

	info.Name = fn.Name
	info.Args = fn.Args
	info.Start = pos + 2
	info.End = cs.len()
	info.Capacity = cs.scopeCapacity()
	if info.Name == "" {
		info.Name = "λ"
	}

	cs.closeFunction()
	cs.scopeDeExtend()

	cs.emit(op.END)

	return pos
}

func (call Call) compile(cs *CompilerState) int {
	pos := cs.emit(op.CALL, byte(len(call.Args)))
	call.Fn.compile(cs)
	for _, arg := range call.Args {
		arg.compile(cs)
	}

	return pos
}

func (g Go) compile(cs *CompilerState) int {
	if call, isCall := g.Routine.(Call); isCall {
		pos := cs.emit(op.GO, byte(len(call.Args)))
		call.Fn.compile(cs)
		for _, arg := range call.Args {
			arg.compile(cs)
		}
		return pos
	} else if call, isCall := g.Routine.(DotCall); isCall {
		pos := call.compile2(cs)
		cs.set(pos, op.GO)
		return pos
	}

	panic("go expected call, got something else")
}

func (ret Return) compile(cs *CompilerState) int {
	pos := cs.emit(op.RET)
	ret.Value.compile(cs)

	return pos
}
