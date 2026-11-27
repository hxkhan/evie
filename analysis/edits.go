package analysis

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing/fstest"
	"unicode"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/vm"
)

// ---------------------------------------------------------------- folding

// FoldingRange is a span of lines an editor can fold; the line that closes it
// stays visible
type FoldingRange struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
}

// Folding lists the bracketed blocks of a file that span lines: bodies,
// switches, and object and array literals. A file with syntax errors folds as
// far as it parsed.
func (w *Workspace) Folding(file string) []FoldingRange {
	text, err := fs.ReadFile(w.fsys, file)
	if err != nil {
		return nil
	}
	node, _ := parser.ParseSource(diag.NewSource(file, text))
	ranges := []FoldingRange{}
	add := func(open, end diag.Pos) {
		// fold from the line that opens to the one before the line that closes
		if open.Known() && end.Known() && end.Line-1 > open.Line {
			ranges = append(ranges, FoldingRange{StartLine: open.Line - 1, EndLine: end.Line - 2})
		}
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case ast.Block:
			add(n.Pos, n.End)
		case ast.Switch:
			add(n.Pos, n.End)
		case ast.Object:
			add(n.Pos, n.End)
		case ast.ArrayLiteral:
			add(n.Pos, n.End)
		}
		return true
	})
	slices.SortFunc(ranges, func(a, b FoldingRange) int { return a.StartLine - b.StartLine })
	return ranges
}

// ---------------------------------------------------------------- highlights

// Highlight kinds, as the Language Server Protocol numbers them
const (
	highlightRead  = 2
	highlightWrite = 3
)

// Highlight marks a place in a file that names the same thing as the cursor
type Highlight struct {
	Range Range `json:"range"`
	Kind  int   `json:"kind"`
}

// Highlights marks every place in file that names what the name at pos
// names; its declaration counts as a write, its uses as reads
func (w *Workspace) Highlights(file string, pos Position) []Highlight {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.names(file)
	r, ok := s.at(file, pos)
	if !ok {
		return nil
	}
	key := declKey(r)
	src := s.source(file)
	marks := []Highlight{}
	for _, other := range s.refs[file] {
		if declKey(other) != key {
			continue
		}
		kind := highlightRead
		if other.Declares {
			kind = highlightWrite
		}
		marks = append(marks, Highlight{Range: toRange(src, other.Pos), Kind: kind})
	}
	return marks
}

// ---------------------------------------------------------------- the whole workspace

// maxFiles is how many .ev files a workspace-wide search looks at
const maxFiles = 5000

// all analyzes every program and package of the workspace afresh, each once,
// for questions whose answer can be in any file, such as where a function of
// a package is used
func (w *Workspace) all() []*snapshot {
	covered := map[string]bool{}
	var snaps []*snapshot
	for _, f := range evFiles(w.fsys) {
		if covered[f] {
			continue
		}
		s := w.update(f)
		for file := range s.files {
			covered[file] = true
		}
		snaps = append(snaps, s)
	}
	return snaps
}

// evFiles lists the .ev files of a workspace, up to maxFiles, leaving out
// hidden folders and node_modules
func evFiles(fsys fs.FS) []string {
	var files []string
	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if p != "." && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".ev") && len(files) < maxFiles {
			files = append(files, p)
		}
		return nil
	})
	return files
}

// everywhere lists the refs of every analysis that name what key names, once each
func everywhere(snaps []*snapshot, key string) []ref {
	seen := map[refKey]bool{}
	var found []ref
	for _, s := range snaps {
		for f, refs := range s.refs {
			for _, r := range refs {
				k := refKey{f, r.Pos.Line, r.Pos.Column, r.Declares}
				if declKey(r) == key && !seen[k] {
					seen[k] = true
					found = append(found, r)
				}
			}
		}
	}
	return found
}

// ---------------------------------------------------------------- rename

// TextEdit replaces a range of a file
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// renamable are the kinds of names whose every use the analysis can find
var renamable = map[vm.NameKind]bool{
	vm.NameVariable: true, vm.NameParameter: true, vm.NameGlobal: true,
	vm.NameFunction: true, vm.NameClass: true, vm.NameImport: true,
}

// PrepareRename tells whether the name at pos can be renamed, and gives its
// range and current text, or the reason it cannot
func (w *Workspace) PrepareRename(file string, pos Position) (Range, string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.names(file)
	r, ok := s.at(file, pos)
	if !ok {
		return Range{}, "", errors.New("there is no name here to rename")
	}
	if err := canRename(r); err != nil {
		return Range{}, "", err
	}
	if r.Kind == vm.NameImport && r.Declares && isQuoted(s, r) {
		return Range{}, "", errors.New("rename the package where it is used, or give the import an alias")
	}
	return toRange(s.source(file), r.Pos), r.Name, nil
}

func canRename(r ref) error {
	switch {
	case r.Kind == vm.NameField:
		return errors.New("a field cannot be renamed yet: it can be used through any value, and those uses cannot all be found")
	case r.Kind == vm.NameMember && (r.DeclSource == nil || !r.DeclPos.Known()):
		return fmt.Errorf("'%s' is defined by the host, not in Evie code", r.Name)
	case r.Kind == vm.NamePrelude:
		return fmt.Errorf("'%s' is a builtin", r.Name)
	case r.Kind == vm.NameMember:
		return nil // a member of an Evie package is renamed where it is declared
	case !renamable[r.Kind]:
		return fmt.Errorf("'%s' cannot be renamed", r.Name)
	}
	return nil
}

// isQuoted reports whether a ref is written as a quoted import path, as the
// declaration of an import without an alias is
func isQuoted(s *snapshot, r ref) bool {
	line, _ := s.source(r.path).Line(r.Pos.Line)
	return r.Pos.Column-1 < len(line) && line[r.Pos.Column-1] == '"'
}

// Rename gives the edits, per file, that rename what the name at pos names to
// newName everywhere in the workspace. It refuses when the code that uses the
// name has errors, and when the renamed code would mean something else, as
// when the new name is taken or shadows another: the renamed code is checked
// before the edits are given.
func (w *Workspace) Rename(file string, pos Position, newName string) (map[string][]TextEdit, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !isIdentifier(newName) {
		return nil, fmt.Errorf("'%s' is not a valid name", newName)
	}
	if slices.Contains(keywords, newName) {
		return nil, fmt.Errorf("'%s' is a keyword", newName)
	}
	r, ok := w.names(file).at(file, pos)
	if !ok {
		return nil, errors.New("there is no name here to rename")
	}
	if err := canRename(r); err != nil {
		return nil, err
	}
	if r.Name == newName {
		return map[string][]TextEdit{}, nil
	}

	// every use in the workspace, from a fresh analysis of everything
	snaps := w.all()
	key := declKey(r)
	targets := everywhere(snaps, key)
	declared := false
	for _, t := range targets {
		declared = declared || t.Declares
	}
	if !declared {
		return nil, fmt.Errorf("the declaration of '%s' is not in this workspace", r.Name)
	}
	var involved []*snapshot
	for _, s := range snaps {
		if len(everywhere([]*snapshot{s}, key)) > 0 {
			involved = append(involved, s)
			for f, diags := range s.diags {
				if HasErrors(diags) {
					return nil, fmt.Errorf("fix the errors in %s first: renaming needs every use of '%s' to be known", f, r.Name)
				}
			}
		}
	}

	// the edits, in the bytes of each file and as ranges for the editor
	byFile := map[string][]byteEdit{}
	for _, t := range targets {
		src := sourceOf(involved, t.path)
		e := byteEdit{line: t.Pos.Line, col: t.Pos.Column, length: len(t.Name), text: newName}
		if t.Kind == vm.NameImport && t.Declares && isQuotedIn(src, t.Pos) {
			// an import without an alias gets one
			e = byteEdit{line: t.Pos.Line, col: t.Pos.Column + t.Pos.Length, text: " as " + newName}
		}
		byFile[t.path] = append(byFile[t.path], e)
	}

	// check the renamed code: no new errors, and every renamed place still
	// names the renamed declaration, and nothing else does
	overlay := fstest.MapFS{}
	for f, edits := range byFile {
		overlay[f] = &fstest.MapFile{Data: apply(sourceOf(involved, f).Text, edits)}
	}
	changed := overlayFS{base: w.fsys, files: overlay}
	var after []*snapshot
	for _, s := range involved {
		after = append(after, analyze(changed, s.entry, w.Snippets))
	}
	for _, s := range after {
		for f, diags := range s.diags {
			if d, ok := firstError(diags); ok {
				return nil, fmt.Errorf("renaming to '%s' would break %s: %s", newName, f, d.Message)
			}
		}
	}
	declAt := ref{}
	for _, t := range targets {
		if t.Declares {
			declAt = t
		}
	}
	newDecl := moved(byFile[declAt.path], declAt.Pos.Line, declAt.Pos.Column)
	if declAt.Kind == vm.NameImport && isQuotedIn(sourceOf(involved, declAt.path), declAt.Pos) {
		newDecl = moved(byFile[declAt.path], declAt.Pos.Line, declAt.Pos.Column+declAt.Pos.Length) + len(" as ")
	}
	newKey := fmt.Sprintf("%s:%d:%d", declAt.path, declAt.Pos.Line, newDecl)
	if got, want := len(everywhere(after, newKey)), len(targets); got != want {
		return nil, fmt.Errorf("renaming to '%s' would change what other code refers to, as when the name is already in use nearby", newName)
	}

	edits := map[string][]TextEdit{}
	for f, list := range byFile {
		src := sourceOf(involved, f)
		for _, e := range list {
			start := toPosition(src, e.line, e.col)
			end := toPosition(src, e.line, e.col+e.length)
			edits[f] = append(edits[f], TextEdit{Range: Range{start, end}, NewText: e.text})
		}
	}
	return edits, nil
}

// byteEdit replaces length bytes at a 1-based line and byte column with text
type byteEdit struct {
	line, col, length int
	text              string
}

// apply makes edits to text; edits do not overlap
func apply(text []byte, edits []byteEdit) []byte {
	lines := strings.Split(string(text), "\n")
	sorted := slices.Clone(edits)
	// from the end of each line backwards, so earlier columns stay put
	slices.SortFunc(sorted, func(a, b byteEdit) int {
		if a.line != b.line {
			return a.line - b.line
		}
		return b.col - a.col
	})
	for _, e := range sorted {
		if e.line-1 >= len(lines) {
			continue
		}
		l := lines[e.line-1]
		start := min(e.col-1, len(l))
		end := min(start+e.length, len(l))
		lines[e.line-1] = l[:start] + e.text + l[end:]
	}
	return []byte(strings.Join(lines, "\n"))
}

// moved is where a column of a line ends up after edits
func moved(edits []byteEdit, line, col int) int {
	shift := 0
	for _, e := range edits {
		if e.line == line && e.col < col {
			shift += len(e.text) - e.length
		}
	}
	return col + shift
}

func isQuotedIn(src *diag.Source, p diag.Pos) bool {
	line, _ := src.Line(p.Line)
	return p.Column-1 < len(line) && line[p.Column-1] == '"'
}

// sourceOf is a file's text in the first analysis that read it
func sourceOf(snaps []*snapshot, file string) *diag.Source {
	for _, s := range snaps {
		if src, ok := s.files[file]; ok {
			return src
		}
	}
	return diag.NewSource(file, nil)
}

func isIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

// overlayFS is a file system with some files replaced
type overlayFS struct {
	base  fs.FS
	files fstest.MapFS
}

func (o overlayFS) Open(name string) (fs.File, error) {
	if _, ok := o.files[name]; ok {
		return o.files.Open(name)
	}
	return o.base.Open(name)
}

func (o overlayFS) ReadFile(name string) ([]byte, error) {
	if f, ok := o.files[name]; ok {
		return f.Data, nil
	}
	return fs.ReadFile(o.base, name)
}

func (o overlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(o.base, name)
}

func (o overlayFS) Stat(name string) (fs.FileInfo, error) {
	if _, ok := o.files[name]; ok {
		return o.files.Stat(name)
	}
	return fs.Stat(o.base, name)
}
