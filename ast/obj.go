package ast

import (
	"fmt"
	"strings"

	"github.com/hxkhan/evie/token"
)

type ObjectField struct {
	Key   string
	Value Node
}

func (node ObjectField) String() string {
	return fmt.Sprintf("%s: %v", node.Key, node.Value)
}

type Object struct {
	token.Pos
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
	token.Pos
	Value Node
}

func (node Exists) String() string {
	return fmt.Sprintf("%v?", node.Value)
}

type Subscript struct {
	token.Pos
	Lhs Node
	Key Node
}

func (node Subscript) String() string {
	return fmt.Sprintf("%v[%v]?", node.Lhs, node.Key)
}
