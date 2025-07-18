package ast

import (
	"slices"

	"github.com/hxkhan/evie/token"
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
	Lhs       Node // [required]
	token.Pos      // [required]
	Operator       // [required]
	Rhs       Node // [required]
}

func (bop BinOp) IsLike(ops ...Operator) bool {
	return slices.Contains(ops, bop.Operator)
}

type Neg struct {
	token.Pos      // [required]
	O         Node // [required]
}
