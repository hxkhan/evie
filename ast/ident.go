package ast

import "github.com/hxkhan/evie/token"

type Decl struct {
	token.Pos
	Name  string
	Value Node
}

type Ident struct {
	token.Pos
	Name string
}

type Assign struct {
	token.Pos
	Lhs   Node
	Value Node
}

func (node Ident) String() string {
	return node.Name
}
