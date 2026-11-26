package ast

import (
	"fmt"
	"github.com/hxkhan/evie/diag"
	"slices"
)

// Operator is a binary operator
type Operator int

// String returns the operator as written, such as "+" or "&&"
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
	case NotEqOp:
		return "!="
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

// The binary operators: arithmetic, then comparison, then logical
const (
	// the vm relies on + - * / coming first, in this order
	AddOp Operator = iota + 1
	SubOp
	MulOp
	DivOp
	ModOp

	EqOp
	NotEqOp
	LtOp
	GtOp
	LtEqOp
	GtEqOp

	OrOp
	AndOp
)

// BinOp is a binary expression such as `a + b` or `a < b`
type BinOp struct {
	diag.Pos // [required]
	Operator // [required]

	Lhs Node // [required]
	Rhs Node // [required]
}

// String returns the expression as Evie source
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
	case NotEqOp:
		return fmt.Sprintf("%v != %v", node.Lhs, node.Rhs)
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

// IsLike reports whether the expression's operator is one of ops
func (bop BinOp) IsLike(ops ...Operator) bool {
	return slices.Contains(ops, bop.Operator)
}

// MutableBinOp is a compound assignment such as `x += 1`
type MutableBinOp struct {
	diag.Pos // [required]
	Operator // [required]

	Lhs Node // [required] can be e.g. Ident
	Rhs Node // [required] can be e.g. Input[float64]
}

// String returns the assignment as Evie source
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

// Plus is unary `+value`
type Plus struct {
	diag.Pos      // [required]
	Value    Node // [required]
}

// Neg is unary `-value`
type Neg struct {
	diag.Pos      // [required]
	Value    Node // [required]
}

// Not is `!value`
type Not struct {
	diag.Pos      // [required]
	Value    Node // [required]
}

// String returns the expression as Evie source
func (node Not) String() string {
	return fmt.Sprintf("!%v", node.Value)
}
