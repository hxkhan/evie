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

type ClassDefinition struct {
	token.Pos
	Name   string
	Parent string
	Args   []string
	Action Node
}

func (node ClassDefinition) String() string {
	return "class"
}

type FieldAccess struct {
	token.Pos
	Lhs Node
	Rhs string
}

func (fa FieldAccess) String() string {
	return fmt.Sprintf("%v.%s", fa.Lhs, fa.Rhs)
}
