package ast

import (
	"fmt"
	"slices"

	"hxkhan.dev/evie/token"
)

type Operator int

func (op Operator) String() string {
	switch op {
	case AddOp:
		return "+"
	case SubOp:
		return "-"
	case MulOp:
		return "*"
	case DivOp:
		return "/"
	case ModOp:
		return "%"

	case EqOp:
		return "=="
	case LtOp:
		return "<"
	case GtOp:
		return ">"
	case LtEqOp:
		return "<="
	case GtEqOp:
		return ">="
	case OrOp:
		return "||"
	case AndOp:
		return "&&"

	}

	return "unknown"
}

const (
	AddOp Operator = iota + 1
	SubOp
	MulOp
	DivOp
	ModOp

	EqOp
	LtOp
	GtOp
	LtEqOp
	GtEqOp

	OrOp
	AndOp
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
	case LtEqOp:
		return fmt.Sprintf("%v <= %v", node.Lhs, node.Rhs)
	case GtEqOp:
		return fmt.Sprintf("%v >= %v", node.Lhs, node.Rhs)

	case OrOp:
		return fmt.Sprintf("%v || %v", node.Lhs, node.Rhs)
	case AndOp:
		return fmt.Sprintf("%v && %v", node.Lhs, node.Rhs)
	}

	return "unknown"
}

func (bop BinOp) IsLike(ops ...Operator) bool {
	return slices.Contains(ops, bop.Operator)
}

type MutableBinOp struct {
	token.Pos // [required]
	Operator  // [required]

	Lhs Node // [required] can be e.g. Ident
	Rhs Node // [required] can be e.g. Input[float64]
}

func (node MutableBinOp) String() string {
	switch node.Operator {
	case AddOp:
		return fmt.Sprintf("%v += %v", node.Lhs, node.Rhs)
	case SubOp:
		return fmt.Sprintf("%v -= %v", node.Lhs, node.Rhs)
	case MulOp:
		return fmt.Sprintf("%v *= %v", node.Lhs, node.Rhs)
	case DivOp:
		return fmt.Sprintf("%v /= %v", node.Lhs, node.Rhs)
	case ModOp:
		return fmt.Sprintf("%v %%= %v", node.Lhs, node.Rhs)
	}

	return "unknown"
}

type Neg struct {
	token.Pos      // [required]
	Value     Node // [required]
}
