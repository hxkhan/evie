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
	Name      string
	NamePos   diag.Pos // where the name is written; unknown for an unnamed class
	Parent    string
	ParentPos diag.Pos // where the parent's name is written, when there is one
	Args      []string
	ArgPos    []diag.Pos // where each of Args is written
	Action    Node
}

func (node ClassDefinition) String() string {
	return "class"
}

type FieldAccess struct {
	diag.Pos
	Lhs    Node
	Rhs    string
	RhsPos diag.Pos // where the member's name is written
}

func (fa FieldAccess) String() string {
	return fmt.Sprintf("%v.%s", fa.Lhs, fa.Rhs)
}
