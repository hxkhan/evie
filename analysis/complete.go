package analysis

import (
	"bytes"
	"regexp"
	"slices"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/vm"
)

// Completion kinds, as the Language Server Protocol numbers them
const (
	completionMethod   = 2
	completionFunction = 3
	completionField    = 5
	completionVariable = 6
	completionClass    = 7
	completionModule   = 9
	completionKeyword  = 14
	completionConstant = 21
)

// Completion is a word that can go where the cursor is
type Completion struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"` // Markdown
}

var keywords = []string{
	"package", "imports", "as", "var", "const", "fn", "class", "super", "this", "echo", "return",
	"if", "else", "switch", "case", "default", "is", "await", "go", "catch", "throw",
	"while", "break", "continue", "for", "in", "nil", "true", "false",
}

// the types every script can name without an import
var builtinTypes = []string{"array", "bool", "buffer", "error", "number", "object", "string", "task"}

// memberAccess matches the end of a line that names a member: `name.` or `name.pre`
var memberAccess = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*\.\s*[A-Za-z0-9_]*$`)

// Complete lists what can be written at pos in path, where text is the
// document as it is now, which may not parse: members after `pkg.` or
// `this.`, and otherwise keywords and the names in scope
func (w *Workspace) Complete(file string, pos Position, text string) []Completion {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.names(file)

	line, col := fromPosition([]byte(text), pos)
	lines := bytes.Split([]byte(text), []byte{'\n'})
	prefix := ""
	if line-1 < len(lines) {
		l := lines[line-1]
		prefix = string(l[:min(col-1, len(l))])
	}

	if m := memberAccess.FindStringSubmatch(prefix); m != nil {
		if m[1] == "this" {
			return s.classMembers(file, line)
		}
		if pkg := s.importNamed(file, m[1]); pkg != nil {
			return packageMembers(pkg)
		}
		return []Completion{} // a member of a value the analysis cannot tell
	}

	var items []Completion
	seen := map[string]bool{}
	add := func(c Completion) {
		if !seen[c.Label] {
			seen[c.Label] = true
			items = append(items, c)
		}
	}
	for _, name := range s.visibleNames(file, line, col) {
		add(name)
	}
	for _, kw := range keywords {
		add(Completion{Label: kw, Kind: completionKeyword})
	}
	for _, t := range builtinTypes {
		add(Completion{Label: t, Kind: completionClass, Detail: "builtin type"})
	}
	return items
}

// importNamed is the package that name is bound to in the package of file
func (s *snapshot) importNamed(file, name string) *vm.Package {
	pkg := s.packageOf(file)
	for f, refs := range s.refs {
		if s.packageOf(f) != pkg {
			continue
		}
		for _, r := range refs {
			if r.Name == name && r.Kind == vm.NameImport && r.Package != nil {
				return r.Package
			}
		}
	}
	return nil
}

// packageMembers lists what a package offers
func packageMembers(pkg *vm.Package) []Completion {
	items := []Completion{}
	for _, name := range pkg.Members() {
		m, ok := pkg.Describe(name)
		if !ok {
			continue
		}
		c := Completion{Label: name, Kind: completionVariable, Detail: m.Signature, Documentation: m.Doc}
		switch m.Value.TypeOf() {
		case "function":
			c.Kind = completionFunction
		case "class":
			c.Kind = completionClass
		case "package":
			c.Kind = completionModule
		case "number", "string", "bool":
			c.Kind = completionConstant
		}
		if m.Yields {
			c.Documentation += "\n\n**yields**: other tasks run while it waits"
		}
		items = append(items, c)
	}
	return items
}

// classMembers lists the fields and methods of the class whose body holds
// line of file, and of its parents in the same package
func (s *snapshot) classMembers(file string, line int) []Completion {
	classes := map[string]ast.ClassDefinition{}
	var inside *ast.ClassDefinition
	for f := range s.files {
		if s.packageOf(f) != s.packageOf(file) {
			continue
		}
		ast.Inspect(s.tree(f), func(n ast.Node) bool {
			cls, ok := n.(ast.ClassDefinition)
			if !ok {
				return true
			}
			classes[cls.Name] = cls
			if body, ok := cls.Action.(ast.Block); ok && f == file && body.Pos.Line <= line && line <= body.End.Line {
				inside = &cls
			}
			return false
		})
	}
	items := []Completion{}
	if inside == nil {
		return items
	}
	seen := map[string]bool{}
	for cls, ok := *inside, true; ok; cls, ok = classes[cls.Parent] {
		for _, field := range cls.Args {
			if !seen[field] {
				seen[field] = true
				items = append(items, Completion{Label: field, Kind: completionField, Detail: "field of " + cls.Name})
			}
		}
		if body, ok := cls.Action.(ast.Block); ok {
			for _, member := range body.Code {
				switch m := member.(type) {
				case ast.Decl:
					if !seen[m.Name] {
						seen[m.Name] = true
						items = append(items, Completion{Label: m.Name, Kind: completionField, Detail: "field of " + cls.Name})
					}
				case ast.Fn:
					if !seen[m.Name] {
						seen[m.Name] = true
						items = append(items, Completion{Label: m.Name, Kind: completionMethod, Detail: fnSignature(m, cls.Name+".")})
					}
				}
			}
		}
		if cls.Parent == "" {
			break
		}
	}
	return items
}

// visibleNames lists the names declared in the package of file and those
// declared before line:col in the blocks around it
func (s *snapshot) visibleNames(file string, line, col int) []Completion {
	blocks := s.blocks(file)
	// innermost is the smallest block around a place, or nil at the top level
	innermost := func(p diag.Pos) *ast.Block {
		var best *ast.Block
		for i := range blocks {
			b := &blocks[i]
			if !before(p.Line, p.Column, b.Pos) && before(p.Line, p.Column, b.End) {
				if best == nil || !before(b.Pos.Line, b.Pos.Column, best.Pos) {
					best = b
				}
			}
		}
		return best
	}
	cursor := diag.Pos{Line: line, Column: col}
	contains := func(b *ast.Block, p diag.Pos) bool {
		return b == nil || !before(p.Line, p.Column, b.Pos) && before(p.Line, p.Column, b.End)
	}
	// parameters and loop variables are written before the body they belong
	// to; an arrow function's parameters reach to the end of its line
	owned := map[diag.Pos]*ast.Block{}
	ast.Inspect(s.tree(file), func(n ast.Node) bool {
		var names []diag.Pos
		var body ast.Node
		switch n := n.(type) {
		case ast.Fn:
			names, body = n.ArgPos, n.Action
		case ast.For:
			names, body = []diag.Pos{n.IndexPos, n.ItemPos}, n.Action
		default:
			return true
		}
		for _, p := range names {
			if !p.Known() {
				continue
			}
			if b, ok := body.(ast.Block); ok {
				owned[p] = &b
			} else {
				owned[p] = &ast.Block{Pos: p, End: diag.Pos{Line: p.Line, Column: 1 << 30}}
			}
		}
		return true
	})

	pkg := s.packageOf(file)
	var items []Completion
	for f, refs := range s.refs {
		for _, r := range refs {
			if !r.Declares || r.Kind == vm.NameField {
				continue
			}
			c := Completion{Label: r.Name, Kind: completionKind(r), Detail: kindWord(r.Kind)}
			if f != file {
				// another file counts only for what its package declares at the top
				if s.packageOf(f) == pkg && isTopLevel(r.Kind) && innermostIn(s, f, r.Pos) == nil {
					items = append(items, c)
				}
				continue
			}
			block, isOwned := owned[r.Pos]
			if !isOwned {
				block = innermost(r.Pos)
			}
			if block == nil && isTopLevel(r.Kind) {
				items = append(items, c) // declared at the top: visible anywhere in the package
				continue
			}
			if before(r.Pos.Line, r.Pos.Column, cursor) && contains(block, cursor) {
				items = append(items, c)
			}
		}
	}
	slices.SortFunc(items, func(a, b Completion) int {
		if a.Label < b.Label {
			return -1
		}
		if a.Label > b.Label {
			return 1
		}
		return 0
	})
	return items
}

// blocks lists every block of file, which have a start and an end
func (s *snapshot) blocks(file string) []ast.Block {
	var blocks []ast.Block
	ast.Inspect(s.tree(file), func(n ast.Node) bool {
		if b, ok := n.(ast.Block); ok && b.Pos.Known() && b.End.Known() {
			blocks = append(blocks, b)
		}
		return true
	})
	return blocks
}

// innermostIn reports the block around p in another file, or nil at its top level
func innermostIn(s *snapshot, file string, p diag.Pos) *ast.Block {
	for _, b := range s.blocks(file) {
		if !before(p.Line, p.Column, b.Pos) && before(p.Line, p.Column, b.End) {
			return &b
		}
	}
	return nil
}

func isTopLevel(k vm.NameKind) bool {
	switch k {
	case vm.NameGlobal, vm.NameFunction, vm.NameClass, vm.NameImport:
		return true
	}
	return false
}

func completionKind(r ref) int {
	switch r.Kind {
	case vm.NameFunction:
		return completionFunction
	case vm.NameClass:
		return completionClass
	case vm.NameImport:
		return completionModule
	case vm.NameGlobal:
		if r.IsStatic {
			return completionConstant
		}
	}
	return completionVariable
}
