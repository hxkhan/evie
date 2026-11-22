package ast

import (
	"fmt"
	"github.com/hxkhan/evie/diag"
	"strings"
)

type ObjectField struct {
	Key   string
	Value Node
}

func (node ObjectField) String() string {
	return fmt.Sprintf("%s: %v", node.Key, node.Value)
}

type Object struct {
	diag.Pos
	Fields []ObjectField
}

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

type Exists struct {
	diag.Pos
	Value Node
}

func (node Exists) String() string {
	return fmt.Sprintf("%v?", node.Value)
}

type Subscript struct {
	diag.Pos
	Lhs Node
	Key Node
}

func (node Subscript) String() string {
	return fmt.Sprintf("%v[%v]", node.Lhs, node.Key)
}

type Slice struct {
	diag.Pos
	Lhs   Node
	Start Node
	End   Node
}

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

type ArrayLiteral struct {
	diag.Pos
	Elements []Node
}

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
