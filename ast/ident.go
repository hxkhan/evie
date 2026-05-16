package ast

import (
	"fmt"

	"github.com/hxkhan/evie/token"
)

type Decl struct {
	token.Pos
	Name     string
	Type     Node
	Value    Node
	IsStatic bool
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

func (node Decl) String() string {
	if node.IsStatic {
		if node.Type != nil {
			return fmt.Sprintf("let %s: %s = %v", node.Name, node.Type, node.Value)
		}
		return fmt.Sprintf("let %s = %v", node.Name, node.Value)
	}
	if node.Type != nil {
		return fmt.Sprintf("var %s: %s = %v", node.Name, node.Type, node.Value)
	}
	return fmt.Sprintf("var %s = %v", node.Name, node.Value)
}

func (node Assign) String() string {
	return fmt.Sprintf("%v = %v", node.Lhs, node.Value)
}
