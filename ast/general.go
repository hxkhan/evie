package ast

import (
	"fmt"
	"github.com/hxkhan/evie/diag"
	"strconv"
	"strings"
)

// Node is any piece of syntax; Where reports its span in the source
type Node interface {
	Where() diag.Pos
}

// ImportSpec is one entry in `imports("path")` or `imports("path" as alias)`.
type ImportSpec struct {
	diag.Pos
	Path  string
	Alias string // empty when not explicitly aliased
}

type Package struct {
	diag.Pos
	Name    string
	Imports []ImportSpec
	Code    []Node
}

type Literal interface {
	bool | float64 | string | struct{}
}

type Input[T Literal] struct {
	diag.Pos
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

type StringTemplate struct {
	diag.Pos
	Format string // e.g. 'Hello %s, how are you!'
	Args   []Node
}

func (node StringTemplate) String() string {
	args := make([]any, len(node.Args))
	for i, a := range node.Args {
		args[i] = fmt.Sprintf("{%v}", a)
	}

	return fmt.Sprintf(node.Format, args...)
}

type Block struct {
	diag.Pos
	Code []Node
}

type Echo struct {
	diag.Pos
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
		for i, spec := range pkg.Imports {
			b.WriteByte('"')
			b.WriteString(spec.Path)
			b.WriteByte('"')
			if spec.Alias != "" {
				b.WriteString(" as ")
				b.WriteString(spec.Alias)
			}

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
