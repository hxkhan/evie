package ast

import (
	"github.com/hk-32/evie/op"
)

type ApplyBinOp struct {
	OP byte
	A  IdentGet // IdentGet/BracketAccess
	B  Node
}

func (bop ApplyBinOp) compile(cs *CompilerState) int {
	pos := cs.emit(opApplyBinOP(bop.OP))
	bop.A.compile(cs)

	// in the future maybe add STORE_ADD_CONST and variants

	// optimise: n += 1 || n -= 1 to inc/dec
	if in, isInput := bop.B.(Input); isInput {
		if in.Value == int64(1) || in.Value == float64(1) {
			switch bop.OP {
			case op.ADD:
				cs.set(pos, op.INC)
				return pos
			case op.SUB:
				cs.set(pos, op.DEC)
				return pos
			}
		}
	}

	bop.B.compile(cs)
	return pos
}

func opApplyBinOP(opcode byte) byte {
	switch opcode {
	case op.ADD:
		return op.STORE_ADD
	case op.SUB:
		return op.STORE_SUB
	}
	panic("opRightConst(op) not implemented for op")
}
