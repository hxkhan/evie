package ast

import (
	"github.com/hk-32/evie/internal/op"
)

/*

var x = something(10)
var y = something(10)

IdentDec{Name: "x", Value: Call{Fn: IdentGet{Name: "something"}, Args: [Input{Value: 10}]}}

*/

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

func (iDec IdentDec) compile(cs *CompilerState) int {
	index := cs.declare(iDec.Name)
	pos := cs.emit(op.STORE_LOCAL, byte(index))
	cs.addReferenceNameFor(pos, iDec.Name)

	iDec.Value.compile(cs)
	return pos
}

func (iDec IdentDec) compileInGlobal(cs *CompilerState) int {
	index := cs.get(iDec.Name)
	pos := cs.emit(op.STORE_LOCAL, byte(index))
	cs.addReferenceNameFor(pos, iDec.Name)

	iDec.Value.compile(cs)
	return pos
}

func (iGet IdentGet) compile(cs *CompilerState) (pos int) {
	ref := cs.reach(iGet.Name)

	switch {
	case ref.Scroll < 0:
		pos = cs.emit(op.LOAD_BUILTIN, byte(ref.Index))
		cs.addReferenceNameFor(pos, iGet.Name)

	case ref.Scroll == 0:
		pos = cs.emit(op.LOAD_LOCAL, byte(ref.Index))
		cs.addReferenceNameFor(pos, iGet.Name)

	case ref.Scroll > 0:
		pos = cs.emit(op.LOAD_CAPTURED, byte(cs.addToCaptured(ref)))
		cs.addReferenceNameFor(pos, iGet.Name)
	}

	return pos
}

func (iSet IdentSet) compile(cs *CompilerState) (pos int) {
	ref := cs.reach(iSet.Name)

	// optimise: n = n+x or n = n-x
	/* if isBinOpLike(iSet.Value, op.ADD, op.SUB) {
		operation := iSet.Value.(BinOp)

		iGet, isIdentGet := operation.A.(IdentGet)
		in, isInput := operation.B.(Input)

		// if left is not load then try flipping if operation is commutative
		if !isIdentGet && (operation.OP == op.ADD || operation.OP == op.MUL) {
			iGet, isIdentGet = operation.B.(IdentGet)
			in, isInput = operation.A.(Input)
		}

		if isIdentGet && iGet.Name == iSet.Name {
			// n++ or n--
			if isInput && (in.Value == int64(1) || in.Value == float64(1)) {
				if operation.OP == op.ADD {
					cs.emit(op.INC)
				} else {
					cs.emit(op.DEC)
				}
				iGet.compile(cs)
				return
			}

			cs.emit(op.STORE_ADD)
			iGet.compile(cs)
			operation.B.compile(cs)
			return
		}
	} */

	switch {
	case ref.Scroll < 0:
		panic("cannot set the value of a built-in")

	case ref.Scroll == 0:
		pos = cs.emit(op.STORE_LOCAL, byte(ref.Index))
		cs.addReferenceNameFor(pos, iSet.Name)

	case ref.Scroll > 0:
		pos = cs.emit(op.STORE_CAPTURED, byte(cs.addToCaptured(ref)))
		cs.addReferenceNameFor(pos, iSet.Name)
	}

	iSet.Value.compile(cs)
	return pos
}
