package ast

import (
	"fmt"
	"github.com/hxkhan/evie/diag"
	"strings"
)

// ObjectField is one `key: value` entry of an Object literal
type ObjectField struct {
	Key    string
	KeyPos diag.Pos // where the key is written
	Value  Node
}

// String returns the entry as Evie source
func (node ObjectField) String() string {
	return fmt.Sprintf("%s: %v", node.Key, node.Value)
}

// Object is an object literal, `{ key: value, ... }`
type Object struct {
	diag.Pos
	Fields []ObjectField
	End    diag.Pos // the closing '}'
}

// String returns the literal as Evie source
func (node Object) String() string {
	var out strings.Builder

	out.WriteString("{")

	fields := []string{}
	for _, field := range node.Fields {
		fields = append(fields, field.String())
	}

	out.WriteString(strings.Join(fields, ", "))
	out.WriteString("}")

	return out.String()
}

// Exists is `value?`, which reports whether a field or index is present
type Exists struct {
	diag.Pos
	Value Node
}

// String returns the check as Evie source
func (node Exists) String() string {
	return fmt.Sprintf("%v?", node.Value)
}

// Subscript is `Lhs[Key]`, indexing an array, string, buffer or object
type Subscript struct {
	diag.Pos
	Lhs Node
	Key Node
}

// String returns the index expression as Evie source
func (node Subscript) String() string {
	return fmt.Sprintf("%v[%v]", node.Lhs, node.Key)
}

// Slice is `Lhs[Start:End]`; either bound may be nil
type Slice struct {
	diag.Pos
	Lhs   Node
	Start Node
	End   Node
}

// String returns the slice expression as Evie source
func (node Slice) String() string {
	var start, end string
	if node.Start != nil {
		start = fmt.Sprint(node.Start)
	}
	if node.End != nil {
		end = fmt.Sprint(node.End)
	}
	return fmt.Sprintf("%v[%s:%s]", node.Lhs, start, end)
}

// ArrayLiteral is an array literal, `[a, b, c]`
type ArrayLiteral struct {
	diag.Pos
	Elements []Node
	End      diag.Pos // the closing ']'
}

// String returns the literal as Evie source
func (node ArrayLiteral) String() string {
	var out strings.Builder
	out.WriteString("[")
	elements := make([]string, len(node.Elements))
	for i, elem := range node.Elements {
		elements[i] = fmt.Sprint(elem)
	}
	out.WriteString(strings.Join(elements, ", "))
	out.WriteString("]")
	return out.String()
}
