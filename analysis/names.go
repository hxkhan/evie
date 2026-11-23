package analysis

import (
	"fmt"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/vm"
)

// Hover is the text shown for the name under the cursor, in Markdown
type Hover struct {
	Contents string `json:"contents"`
	Range    Range  `json:"range"`
}

// at finds the name at a position of file
func (s *snapshot) at(file string, pos Position) (ref, bool) {
	line, col := fromPosition(s.source(file).Text, pos)
	var touching ref
	found := false
	for _, r := range s.refs[file] {
		if r.Pos.Line != line || col < r.Pos.Column {
			continue
		}
		end := r.Pos.Column + max(r.Pos.Length, 1)
		if col < end {
			return r, true
		}
		if col == end {
			// the cursor just after a name, as when it was just typed
			touching, found = r, true
		}
	}
	return touching, found
}

// declKey names what a ref refers to, the same for its declaration and every use
func declKey(r ref) string {
	switch {
	case r.Declares:
		return fmt.Sprintf("%s:%d:%d", r.path, r.Pos.Line, r.Pos.Column)
	case r.DeclSource != nil && r.DeclPos.Known():
		return fmt.Sprintf("%s:%d:%d", r.DeclSource.Name, r.DeclPos.Line, r.DeclPos.Column)
	case r.Package != nil:
		return "member:" + r.Package.ID() + "." + r.Name
	default:
		return "builtin:" + r.Name
	}
}

// Definition is where the name at pos in path is declared
func (w *Workspace) Definition(file string, pos Position) (Location, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.names(file)
	r, ok := s.at(file, pos)
	if !ok {
		return Location{}, false
	}
	if r.Declares {
		return Location{Path: r.path, Range: toRange(s.source(r.path), r.Pos)}, true
	}
	if r.DeclSource == nil || !r.DeclPos.Known() {
		return Location{}, false // a host function or a builtin has no source
	}
	return Location{Path: r.DeclSource.Name, Range: toRange(r.DeclSource, r.DeclPos)}, true
}

// References are the places in the workspace that name what the name at pos
// in path names, with its declaration when includeDecl is set. Every program
// of the workspace is searched, so the uses of a package's function are found
// in every program that imports it.
func (w *Workspace) References(file string, pos Position, includeDecl bool) []Location {
	w.mu.Lock()
	defer w.mu.Unlock()
	r, ok := w.names(file).at(file, pos)
	if !ok {
		return nil
	}
	snaps := w.all()
	locs := []Location{}
	for _, other := range everywhere(snaps, declKey(r)) {
		if includeDecl || !other.Declares {
			locs = append(locs, Location{Path: other.path, Range: toRange(sourceOf(snaps, other.path), other.Pos)})
		}
	}
	sortLocations(locs)
	return locs
}

// Hover describes the name at pos in path
func (w *Workspace) Hover(file string, pos Position) (Hover, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.names(file)
	r, ok := s.at(file, pos)
	if !ok {
		return Hover{}, false
	}
	var b strings.Builder
	sig, what := s.describe(r)
	if sig != "" {
		fmt.Fprintf(&b, "```evie\n%s\n```\n", sig)
	}
	b.WriteString(what)
	if r.Package != nil && !r.Package.Compiled() {
		// a member of a host package, such as the standard library, tells what it does
		if m, ok := r.Package.Describe(r.Name); ok && r.Kind == vm.NameMember {
			if m.Doc != "" {
				b.WriteString("\n\n" + m.Doc)
			}
			if m.Yields {
				b.WriteString("\n\n**yields**: other tasks run while it waits")
			}
		}
	}
	return Hover{Contents: b.String(), Range: toRange(s.source(file), r.Pos)}, true
}

// describe gives the code that declares what r names and a line saying what it is
func (s *snapshot) describe(r ref) (sig, what string) {
	kind := kindWord(r.Kind)
	declFile, declPos := r.path, r.Pos
	if !r.Declares {
		if r.DeclSource == nil || !r.DeclPos.Known() {
			// declared by the host or the prelude
			if r.Kind == vm.NameMember && r.Package != nil {
				if m, ok := r.Package.Describe(r.Name); ok {
					return fmt.Sprintf("%s.%s", r.Package.Name(), m.Signature), fmt.Sprintf("*%s of package `%s`*", kind, r.Package.Name())
				}
			}
			return r.Name, "*" + kind + "*"
		}
		declFile, declPos = r.DeclSource.Name, r.DeclPos
	}
	sig = s.signature(declFile, declPos, r.Name)
	what = "*" + kind + "*"
	switch {
	case r.Kind == vm.NameImport && r.Package != nil:
		what = fmt.Sprintf("*package `%s`*", r.Package.Name())
	case r.Kind == vm.NameMember && r.Package != nil:
		what = fmt.Sprintf("*%s of package `%s`*", kind, r.Package.Name())
	}
	if declFile != r.path && r.Kind != vm.NameImport {
		what += fmt.Sprintf(" in `%s`", declFile)
	}
	return sig, what
}

// signature is the code that declares the name written at pos of file
func (s *snapshot) signature(file string, pos diag.Pos, name string) string {
	sig := ""
	ast.Inspect(s.tree(file), func(n ast.Node) bool {
		if sig != "" {
			return false
		}
		switch n := n.(type) {
		case ast.Package:
			for _, spec := range n.Imports {
				if spec.Pos == pos || spec.AliasPos == pos {
					sig = fmt.Sprintf("imports(%q)", spec.Path)
					if spec.Alias != "" {
						sig = fmt.Sprintf("imports(%q as %s)", spec.Path, spec.Alias)
					}
				}
			}
		case ast.ClassDefinition:
			switch {
			case n.NamePos == pos:
				sig = classSignature(n)
			case indexOf(n.ArgPos, pos) >= 0:
				sig = fmt.Sprintf("%s (a field of %s)", name, n.Name)
			default:
				if body, ok := n.Action.(ast.Block); ok {
					for _, member := range body.Code {
						switch m := member.(type) {
						case ast.Decl:
							if m.Pos == pos {
								sig = s.lineAt(file, pos) + fmt.Sprintf("  // a field of %s", n.Name)
							}
						case ast.Fn:
							if m.NamePos == pos {
								sig = fnSignature(m, n.Name+".")
							}
						}
					}
				}
			}
		case ast.Fn:
			if n.NamePos == pos {
				sig = fnSignature(n, "")
			} else if i := indexOf(n.ArgPos, pos); i >= 0 {
				sig = fmt.Sprintf("%s (a parameter of %s)", name, fnSignature(n, ""))
			}
		case ast.Decl:
			if n.Pos == pos {
				sig = s.lineAt(file, pos)
			}
		case ast.For:
			if n.IndexPos == pos || n.ItemPos == pos {
				sig = name + " (a loop variable)"
			}
		}
		return true
	})
	if sig == "" {
		return name
	}
	return sig
}

// lineAt is the line of file that pos is on, without its indentation
func (s *snapshot) lineAt(file string, pos diag.Pos) string {
	text, _ := s.source(file).Line(pos.Line)
	line := strings.TrimSpace(string(text))
	if len(line) > 120 {
		line = line[:117] + "..."
	}
	return line
}

func fnSignature(fn ast.Fn, prefix string) string {
	args := fn.Args
	if fn.IsVariadic && len(args) > 0 {
		args = append(args[:len(args)-1:len(args)-1], "..."+args[len(args)-1])
	}
	return fmt.Sprintf("fn %s%s(%s)", prefix, fn.Name, strings.Join(args, ", "))
}

func classSignature(cls ast.ClassDefinition) string {
	sig := fmt.Sprintf("class %s(%s)", cls.Name, strings.Join(cls.Args, ", "))
	if cls.Parent != "" {
		sig += " : " + cls.Parent
	}
	return sig
}

func kindWord(k vm.NameKind) string {
	if k == vm.NamePrelude {
		return "builtin"
	}
	return k.String()
}

func indexOf(positions []diag.Pos, p diag.Pos) int {
	for i, q := range positions {
		if q == p {
			return i
		}
	}
	return -1
}
