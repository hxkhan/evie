package ast

import (
	"fmt"
	"slices"

	"github.com/hxkhan/evie/token"
)

type Operator int

const (
	AddOp Operator = iota + 1
	SubOp
	MulOp
	DivOp
	ModOp

	EqOp
	LtOp
	GtOp
)

type BinOp struct {
	token.Pos // [required]
	Operator  // [required]

	Lhs Node // [required]
	Rhs Node // [required]
}

func (node BinOp) String() string {
	switch node.Operator {
	case AddOp:
		return fmt.Sprintf("%v + %v", node.Lhs, node.Rhs)
	case SubOp:
		return fmt.Sprintf("%v - %v", node.Lhs, node.Rhs)
	case MulOp:
		return fmt.Sprintf("%v * %v", node.Lhs, node.Rhs)
	case DivOp:
		return fmt.Sprintf("%v / %v", node.Lhs, node.Rhs)
	case ModOp:
		return fmt.Sprintf("%v %% %v", node.Lhs, node.Rhs)

	case EqOp:
		return fmt.Sprintf("%v == %v", node.Lhs, node.Rhs)
	case LtOp:
		return fmt.Sprintf("%v < %v", node.Lhs, node.Rhs)
	case GtOp:
		return fmt.Sprintf("%v > %v", node.Lhs, node.Rhs)
	}

	return "unknown"
}

func (bop BinOp) IsLike(ops ...Operator) bool {
	return slices.Contains(ops, bop.Operator)
}

type Neg struct {
	token.Pos      // [required]
	Value     Node // [required]
}
