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
	Path     string
	Alias    string   // empty when not explicitly aliased
	AliasPos diag.Pos // where the alias is written, when there is one
}

// Package is a parsed file: its package clause, its imports and its top-level code
type Package struct {
	diag.Pos
	Name    string
	NamePos diag.Pos // where the package name is written
	Imports []ImportSpec
	Code    []Node
}

// Literal lists the Go types an Input can hold; struct{} stands for nil
type Literal interface {
	bool | float64 | string | struct{}
}

// Input is a literal in the source: a number, string, bool or nil
type Input[T Literal] struct {
	diag.Pos
	Value T
}

// String returns the literal as Evie source
func (node Input[T]) String() string {
	if f, isFloat := any(node.Value).(float64); isFloat {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	if s, isString := any(node.Value).(string); isString {
		return fmt.Sprintf("\"%v\"", s)
	}

	return fmt.Sprint(node.Value)
}

// StringTemplate is a backtick string with `{expr}` interpolations
type StringTemplate struct {
	diag.Pos
	Parts []string // the literal text around the interpolations; one more than Args
	Args  []Node
}

// String returns the template's text with each interpolation in braces
func (node StringTemplate) String() string {
	var b strings.Builder
	b.WriteString(node.Parts[0])
	for i, a := range node.Args {
		fmt.Fprintf(&b, "{%v}", a)
		b.WriteString(node.Parts[i+1])
	}
	return b.String()
}

// Block is code between braces; Pos is its '{' and End its '}'
type Block struct {
	diag.Pos
	Code []Node
	End  diag.Pos
}

// Echo is `echo value`, which prints the value to the engine's standard output
type Echo struct {
	diag.Pos
	Value Node
}

// String returns the statement as Evie source
func (echo Echo) String() string {
	return fmt.Sprintf("echo %v", echo.Value)
}

// String returns the package clause, its imports and its code as Evie source
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

// String returns the block as Evie source
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
