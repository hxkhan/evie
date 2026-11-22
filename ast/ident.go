package ast

import (
	"fmt"
	"github.com/hxkhan/evie/diag"
)

type Decl struct {
	diag.Pos
	Name     string
	Value    Node
	IsStatic bool
}

type Ident struct {
	diag.Pos
	Name string
}

type Assign struct {
	diag.Pos
	Lhs   Node
	Value Node
}

func (node Ident) String() string {
	return node.Name
}

func (node Decl) String() string {
	if node.IsStatic {
		return fmt.Sprintf("%s := %v", node.Name, node.Value)
	}
	return fmt.Sprintf("var %s := %v", node.Name, node.Value)
}
