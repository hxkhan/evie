package analysis

import (
	"io/fs"
	"slices"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
)

// Symbol kinds, as the Language Server Protocol numbers them
const (
	symbolClass    = 5
	symbolMethod   = 6
	symbolField    = 8
	symbolFunction = 12
	symbolVariable = 13
	symbolConstant = 14
)

// Symbol is an entry of a file's outline
type Symbol struct {
	Name           string   `json:"name"`
	Detail         string   `json:"detail,omitempty"`
	Kind           int      `json:"kind"`
	Range          Range    `json:"range"`
	SelectionRange Range    `json:"selectionRange"`
	Children       []Symbol `json:"children,omitempty"`
}

// Symbols outlines a file: its functions, classes with their fields and
// methods, and top-level bindings. A file with syntax errors is outlined as
// far as it parsed.
func (w *Workspace) Symbols(file string) []Symbol {
	text, err := fs.ReadFile(w.fsys, file)
	if err != nil {
		return nil
	}
	src := diag.NewSource(file, text)
	node, _ := parser.ParseSource(src)
	var code []ast.Node
	switch n := node.(type) {
	case ast.Package:
		code = n.Code
	case ast.Block:
		code = n.Code
	}
	symbols := []Symbol{}
	for _, stmt := range code {
		if sym, ok := symbolOf(src, stmt, ""); ok {
			symbols = append(symbols, sym)
		}
	}
	return symbols
}

// symbolOf outlines a top-level statement, or a member of the class className
func symbolOf(src *diag.Source, node ast.Node, className string) (Symbol, bool) {
	switch n := node.(type) {
	case ast.Fn:
		if n.Name == "" || !n.NamePos.Known() {
			return Symbol{}, false
		}
		kind, prefix := symbolFunction, ""
		if className != "" {
			kind, prefix = symbolMethod, className+"."
		}
		return Symbol{
			Name:           n.Name,
			Detail:         strings.TrimPrefix(fnSignature(n, prefix), "fn "+prefix+n.Name),
			Kind:           kind,
			Range:          spanRange(src, n.Pos, endOf(n.Action)),
			SelectionRange: toRange(src, n.NamePos),
		}, true

	case ast.ClassDefinition:
		if n.Name == "" || !n.NamePos.Known() {
			return Symbol{}, false
		}
		sym := Symbol{
			Name:           n.Name,
			Detail:         strings.TrimPrefix(classSignature(n), "class "+n.Name),
			Kind:           symbolClass,
			Range:          spanRange(src, n.Pos, endOf(n.Action)),
			SelectionRange: toRange(src, n.NamePos),
		}
		for i, field := range n.Args {
			if i < len(n.ArgPos) {
				r := toRange(src, n.ArgPos[i])
				sym.Children = append(sym.Children, Symbol{Name: field, Kind: symbolField, Range: r, SelectionRange: r})
			}
		}
		if body, ok := n.Action.(ast.Block); ok {
			for _, member := range body.Code {
				if child, ok := symbolOf(src, member, n.Name); ok {
					if child.Kind == symbolConstant || child.Kind == symbolVariable {
						child.Kind = symbolField
					}
					sym.Children = append(sym.Children, child)
				}
			}
		}
		return sym, true

	case ast.Decl:
		kind := symbolVariable
		if n.IsStatic {
			kind = symbolConstant
		}
		return Symbol{
			Name:           n.Name,
			Kind:           kind,
			Range:          spanRange(src, n.Pos, endOf(n.Value)),
			SelectionRange: toRange(src, n.Pos),
		}, true
	}
	return Symbol{}, false
}

// endOf is the last span of a node that the outline can tell: a block's
// closing brace, or where the node starts
func endOf(node ast.Node) diag.Pos {
	switch n := node.(type) {
	case nil:
		return diag.Pos{}
	case ast.Block:
		if n.End.Known() {
			return n.End
		}
	}
	return node.Where()
}

// sortLocations orders locations by file and place
func sortLocations(locs []Location) {
	slices.SortFunc(locs, func(a, b Location) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		if a.Range.Start.Line != b.Range.Start.Line {
			return a.Range.Start.Line - b.Range.Start.Line
		}
		return a.Range.Start.Character - b.Range.Start.Character
	})
}
