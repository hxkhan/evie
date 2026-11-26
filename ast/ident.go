package ast

import (
	"fmt"
	"github.com/hxkhan/evie/diag"
)

// Decl declares a binding: `x := v` and `const x := v` are static (immutable),
// `var x := v` is not
type Decl struct {
	diag.Pos
	Name     string
	Value    Node
	IsStatic bool
}

// Ident is a name used as a value
type Ident struct {
	diag.Pos
	Name string
}

// Assign is `Lhs = Value`, where Lhs is a name, a field or an index
type Assign struct {
	diag.Pos
	Lhs   Node
	Value Node
}

// String returns the name
func (node Ident) String() string {
	return node.Name
}

// String returns the declaration as Evie source
func (node Decl) String() string {
	if node.IsStatic {
		return fmt.Sprintf("%s := %v", node.Name, node.Value)
	}
	return fmt.Sprintf("var %s := %v", node.Name, node.Value)
}
