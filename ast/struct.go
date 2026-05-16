package ast

import (
	"fmt"

	"github.com/hxkhan/evie/token"
)

type IsInstanceOf struct {
	token.Pos
	Lhs Node
	Rhs Node
}

type StructDefinition struct {
	token.Pos
	Name   string
	Args   []string
	Action Node
}

func (node StructDefinition) String() string {
	return "struct"
}

type FieldAccess struct {
	token.Pos
	Lhs Node
	Rhs string
}

func (fa FieldAccess) String() string {
	return fmt.Sprintf("%v.%s", fa.Lhs, fa.Rhs)
}

func (is IsInstanceOf) String() string {
	return fmt.Sprintf("%v is %v", is.Lhs, is.Rhs)
}
