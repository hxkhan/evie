package ast

import (
	"fmt"
	"github.com/hxkhan/evie/diag"
)

// IsInstanceOf is `Lhs is Rhs`, true when Lhs is of the type or class Rhs names
type IsInstanceOf struct {
	diag.Pos
	Lhs Node
	Rhs Node
}

// ClassDefinition is a class declaration or expression: its constructor
// parameters, its optional parent and the body that declares its fields and methods
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

// String returns "class"
func (node ClassDefinition) String() string {
	return "class"
}

// FieldAccess is `Lhs.Rhs`, reading a field, method or package member by name
type FieldAccess struct {
	diag.Pos
	Lhs    Node
	Rhs    string
	RhsPos diag.Pos // where the member's name is written
}

// String returns the access as written, such as "user.name"
func (fa FieldAccess) String() string {
	return fmt.Sprintf("%v.%s", fa.Lhs, fa.Rhs)
}
