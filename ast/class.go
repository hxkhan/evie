package ast

import (
	"fmt"
	"github.com/hxkhan/evie/diag"
)

type IsInstanceOf struct {
	diag.Pos
	Lhs Node
	Rhs Node
}

type ClassDefinition struct {
	diag.Pos
	Name   string
	Parent string
	Args   []string
	Action Node
}

func (node ClassDefinition) String() string {
	return "class"
}

type FieldAccess struct {
	diag.Pos
	Lhs Node
	Rhs string
}

func (fa FieldAccess) String() string {
	return fmt.Sprintf("%v.%s", fa.Lhs, fa.Rhs)
}
