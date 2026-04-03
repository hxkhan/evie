package ast

import "github.com/hxkhan/evie/token"

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
