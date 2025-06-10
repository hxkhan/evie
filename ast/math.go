package ast

import (
	"github.com/hk-32/evie/op"
)

func isBinOpLike(n Node, ops ...byte) bool {
	if binOp, isBinOp := n.(BinOp); isBinOp {
		for _, op := range ops {
			if binOp.OP == op {
				return true
			}
		}
	}
	return false
}

type BinOp struct {
	OP byte // [required]
	A  Node // [required]
	B  Node // [required]
}

func (bop BinOp) compile(cs *CompilerState) int {
	pos := cs.emit(bop.OP)
	bop.A.compile(cs)
	bop.B.compile(cs)

	// optimise: n + literal
	if in, isInput := bop.B.(Input); isInput && cs.optimise {
		switch in.Value.(type) {
		case int64, float64:
			cs.set(pos, opRightConst(bop.OP))
		}
	}

	return pos
}

func opRightConst(opcode byte) byte {
	switch opcode {
	case op.ADD:
		return op.ADD_RIGHT_CONST
	case op.SUB:
		return op.SUB_RIGHT_CONST
	case op.DIV:
		return op.DIV
	case op.MUL:
		return op.MUL
	case op.LS:
		return op.LS_RIGHT_CONST
	case op.EQ:
		return op.EQ
	case op.MR:
		return op.MR
	}
	panic("opRightConst(op) not implemented for op")
}

type Neg struct {
	O Node // [required]
}

func (neg Neg) compile(cs *CompilerState) (pos int) {
	if in, isInput := neg.O.(Input); isInput {
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

	return pos
}
