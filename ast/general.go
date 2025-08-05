package ast

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hxkhan/evie/token"
)

type Node interface {
	Line() int
}

type Package struct {
	token.Pos
	Name    string
	Imports []string
	Code    []Node
}

type Literal interface {
	bool | float64 | string | struct{}
}

type Input[T Literal] struct {
	token.Pos
	Value T
}

func (node Input[T]) String() string {
	if f, isFloat := any(node.Value).(float64); isFloat {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	if s, isString := any(node.Value).(string); isString {
		return fmt.Sprintf("\"%v\"", s)
	}

	return fmt.Sprint(node.Value)
}

type Block struct {
	token.Pos
	Code []Node
}

type Echo struct {
	token.Pos
	Value Node
}

func (echo Echo) String() string {
	return fmt.Sprintf("echo %v", echo.Value)
}

func (pkg Package) String() string {
	b := strings.Builder{}
	b.WriteString("package ")
	b.WriteString(pkg.Name)

	// imports
	if len(pkg.Imports) > 0 {
		b.WriteString(" imports(")
		for i, name := range pkg.Imports {
			b.WriteByte('"')
			b.WriteString(name)
			b.WriteByte('"')

			if i != len(pkg.Imports)-1 {
				b.WriteByte(',')
				b.WriteByte(' ')
			}
		}
		b.WriteByte(')')
	}

	// code
	b.WriteByte('\n')
	for i, node := range pkg.Code {
		b.WriteString(fmt.Sprint(node))

		if i != len(pkg.Code)-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func (block Block) String() string {
	b := strings.Builder{}
	b.WriteString("{\n")

	// code
	for _, node := range block.Code {
		b.WriteString(fmt.Sprint(node))
		b.WriteByte('\n')
	}

	b.WriteString("}\n")

	return b.String()
}
