// Package analysis checks Evie code without running it and answers what an
// editor asks about it: errors, an outline, where a name is declared and used,
// hover text and completions. It is the core of Evie's language server.
//
// Files are read through an fs.FS whose paths are slash-separated and relative
// to the workspace root, so an editor can serve its unsaved documents.
package analysis

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/vm"
)

// Diagnostic is an error in a file, or a hint about it, the way the Language
// Server Protocol describes one
type Diagnostic struct {
	Range    Range     `json:"range"`
	Severity int       `json:"severity"`
	Source   string    `json:"source"`
	Message  string    `json:"message"`
	Related  []Related `json:"relatedInformation,omitempty"`
	Tags     []int     `json:"tags,omitempty"`
	Data     *Fixes    `json:"data,omitempty"`
}

// Diagnostic severities and tags, as the Language Server Protocol numbers them
const (
	SeverityError  = 1
	SeverityHint   = 4
	tagUnnecessary = 1 // the editor fades the code out
)

// Fixes are the edits that would correct a diagnostic, offered as quick fixes
type Fixes struct {
	Fixes []Fix `json:"fixes"`
}

// Fix is one way to correct a diagnostic, by an edit of its file
type Fix struct {
	Title string   `json:"title"`
	Edit  TextEdit `json:"edit"`
}

// HasErrors reports whether diags hold an error, rather than only hints
func HasErrors(diags []Diagnostic) bool {
	_, ok := firstError(diags)
	return ok
}

func firstError(diags []Diagnostic) (Diagnostic, bool) {
	for _, d := range diags {
		if d.Severity == SeverityError {
			return d, true
		}
	}
	return Diagnostic{}, false
}

// Related is a place that explains a diagnostic, such as where a bracket opens
type Related struct {
	Location Location `json:"location"`
	Message  string   `json:"message"`
}

// Workspace answers questions about the Evie files of a folder. It keeps, for
// every file, the latest analysis that covered it and the latest one in which
// it parsed, so that while code is being typed and does not parse, going to a
// definition or completing a name still works from the last good version.
type Workspace struct {
	// Snippets checks a file without a package header the way a session runs
	// it, as the playground and evie -e do: a standard package it names, such
	// as io in io.println, is imported on first use. Set it before the first
	// question. Otherwise such a file is a script, which imports what it uses.
	Snippets bool

	mu     sync.Mutex
	fsys   fs.FS
	latest map[string]*snapshot
	good   map[string]*snapshot
	stdlib map[string]*vm.Package // standard packages a snippet names, made when first asked for
}

// NewWorkspace reads files from fsys
func NewWorkspace(fsys fs.FS) *Workspace {
	return &Workspace{fsys: fsys, latest: map[string]*snapshot{}, good: map[string]*snapshot{}}
}

// Update analyzes the program or package that the file at path belongs to,
// and returns the diagnostics of every file the analysis covered; a file
// without any has an empty list, so an editor can clear old ones
func (w *Workspace) Update(path string) map[string][]Diagnostic {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.update(path).diags
}

func (w *Workspace) update(path string) *snapshot {
	s := analyze(w.fsys, path, w.Snippets)
	for file := range s.files {
		w.latest[file] = s
		if s.parsed {
			w.good[file] = s
		}
	}
	return s
}

// names is the analysis to answer questions about names in path from
func (w *Workspace) names(path string) *snapshot {
	if s, ok := w.good[path]; ok {
		return s
	}
	if s, ok := w.latest[path]; ok {
		return s
	}
	return w.update(path)
}

// snapshot is one analysis of a program or package
type snapshot struct {
	entry  string
	parsed bool                    // every file parsed, so the program was compiled and its names are known
	files  map[string]*diag.Source // every file the analysis read
	diags  map[string][]Diagnostic
	refs   map[string][]ref // every name declared or used, by file
	seen   map[refKey]bool
	trees  map[string]ast.Node // parsed files, made when needed
}

// ref is a name the compiler declared or resolved
type ref struct {
	vm.NameEvent
	path string
}

type refKey struct {
	path      string
	line, col int
	declares  bool
}

func analyze(fsys fs.FS, path string, snippets bool) *snapshot {
	s := &snapshot{
		entry: path,
		files: map[string]*diag.Source{},
		diags: map[string][]Diagnostic{},
		refs:  map[string][]ref{},
		seen:  map[refKey]bool{},
		trees: map[string]ast.Node{},
	}
	s.read(fsys, path)

	b, err := parseFor(fsys, path)
	if err != nil {
		s.fail(err)
		return s
	}
	s.parsed = true
	for _, file := range b.Files() {
		s.read(fsys, file)
	}

	eng := evie.New(evie.Options{FS: fsys, Stdin: strings.NewReader(""), Stdout: io.Discard, OnName: s.record})
	defer eng.Close()
	check := func() error { return eng.Check(b) }
	if src := s.files[path]; snippets && parser.PeekPackageName(src) == "" {
		check = func() error { return eng.NewSession().CompileSource(path, src.Text) }
	}
	if err := check(); err != nil {
		s.fail(err)
	} else {
		s.unused()
	}
	return s
}

// unused adds a hint for every local variable and import nothing uses. It
// runs only on code that compiled, as then every use is known. A name that
// starts with _ is unused on purpose.
func (s *snapshot) unused() {
	used := map[string]bool{}
	for _, refs := range s.refs {
		for _, r := range refs {
			if !r.Declares {
				used[declKey(r)] = true
			}
		}
	}
	for file, refs := range s.refs {
		for _, r := range refs {
			if !r.Declares || used[declKey(r)] || strings.HasPrefix(r.Name, "_") {
				continue
			}
			var msg string
			switch r.Kind {
			case vm.NameVariable:
				msg = fmt.Sprintf("'%s' is declared but never used", r.Name)
			case vm.NameImport:
				msg = fmt.Sprintf("'%s' is imported but never used", r.Name)
			default:
				continue
			}
			s.diags[file] = append(s.diags[file], Diagnostic{
				Range: toRange(s.source(file), r.Pos), Severity: SeverityHint, Source: "evie", Message: msg, Tags: []int{tagUnnecessary},
			})
		}
	}
	for file, diags := range s.diags {
		slices.SortFunc(diags, func(a, b Diagnostic) int {
			if a.Range.Start.Line != b.Range.Start.Line {
				return a.Range.Start.Line - b.Range.Start.Line
			}
			return a.Range.Start.Character - b.Range.Start.Character
		})
		s.diags[file] = diags
	}
}

// parseFor parses what the file at path belongs to: a script on its own, the
// project of a main package's folder, the package of a folder whose files all
// declare it, or a file that is a package by itself
func parseFor(fsys fs.FS, file string) (*evie.Bundle, error) {
	text, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, err
	}
	name := parser.PeekPackageName(diag.NewSource(file, text))
	dir := path.Dir(file)
	switch {
	case name == "":
		return evie.ParseFile(fsys, file)
	case name == "main" && !separatePrograms(fsys, dir):
		return evie.ParseProject(fsys, dir)
	case name == "main":
		return evie.ParseFile(fsys, file)
	case folderIsPackage(fsys, dir, name):
		return evie.ParsePackage(fsys, dir)
	default:
		return evie.ParseFile(fsys, file)
	}
}

// separatePrograms reports whether the main files of dir are programs of their
// own, as in a folder of examples, rather than one program split over files:
// more than one of them declares fn main
func separatePrograms(fsys fs.FS, dir string) bool {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return false
	}
	mains := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ev") {
			continue
		}
		text, err := fs.ReadFile(fsys, path.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		src := diag.NewSource(entry.Name(), text)
		if parser.PeekPackageName(src) != "main" {
			continue
		}
		node, _ := parser.ParseSource(src)
		if pkg, ok := node.(ast.Package); ok {
			for _, stmt := range pkg.Code {
				if fn, ok := stmt.(ast.Fn); ok && fn.Name == "main" && !fn.UsedAsExpr {
					mains++
					break
				}
			}
		}
		if mains > 1 {
			return true
		}
	}
	return false
}

// folderIsPackage reports whether every .ev file in dir declares package name
func folderIsPackage(fsys fs.FS, dir, name string) bool {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ev") {
			continue
		}
		text, err := fs.ReadFile(fsys, path.Join(dir, entry.Name()))
		if err != nil || parser.PeekPackageName(diag.NewSource(entry.Name(), text)) != name {
			return false
		}
	}
	return true
}

// read adds a file of the workspace to the analysis
func (s *snapshot) read(fsys fs.FS, file string) *diag.Source {
	if src, ok := s.files[file]; ok {
		return src
	}
	text, _ := fs.ReadFile(fsys, file)
	src := diag.NewSource(file, text)
	s.files[file] = src
	if _, ok := s.diags[file]; !ok {
		s.diags[file] = []Diagnostic{}
	}
	return src
}

// source is the text the analysis knows file by
func (s *snapshot) source(file string) *diag.Source {
	if src, ok := s.files[file]; ok {
		return src
	}
	return diag.NewSource(file, nil)
}

// record keeps a name the compiler reported, once
func (s *snapshot) record(e vm.NameEvent) {
	if e.Source == nil || !e.Pos.Known() {
		return
	}
	key := refKey{e.Source.Name, e.Pos.Line, e.Pos.Column, e.Declares}
	if s.seen[key] {
		return
	}
	s.seen[key] = true
	if _, ok := s.files[e.Source.Name]; !ok {
		s.files[e.Source.Name] = e.Source
		s.diags[e.Source.Name] = []Diagnostic{}
	}
	s.refs[e.Source.Name] = append(s.refs[e.Source.Name], ref{NameEvent: e, path: e.Source.Name})
}

// fail turns an error from parsing or checking into diagnostics
func (s *snapshot) fail(err error) {
	reports := reportsOf(err)
	if len(reports) == 0 {
		// an error without a place, such as an import cycle, belongs to the entry
		s.diags[s.entry] = append(s.diags[s.entry], Diagnostic{Severity: SeverityError, Source: "evie", Message: err.Error()})
		return
	}
	for _, r := range reports {
		file := s.entry
		src := s.source(s.entry)
		if r.Source != nil && r.Source.Name != "" {
			file, src = r.Source.Name, r.Source
			if _, ok := s.files[file]; !ok {
				s.files[file] = src
			}
		}
		d := Diagnostic{Range: toRange(src, r.Pos), Severity: SeverityError, Source: "evie", Message: message(r), Data: fixesOf(src, r)}
		for _, n := range r.Notes {
			if n.Source == nil {
				continue
			}
			d.Related = append(d.Related, Related{
				Location: Location{Path: n.Source.Name, Range: toRange(n.Source, n.Pos)},
				Message:  n.Text,
			})
		}
		s.diags[file] = append(s.diags[file], d)
	}
}

var (
	quotedName  = regexp.MustCompile(`'([A-Za-z_][A-Za-z0-9_]*)'`)
	similarName = regexp.MustCompile(`similar name(?: exists)?: '([A-Za-z_][A-Za-z0-9_]*)'`)
)

// fixesOf offers the name the compiler suggests for one it cannot find, as in
// "cannot find 'pritnln'" with the hint of a similar name 'println'
func fixesOf(src *diag.Source, r *diag.Report) *Fixes {
	bad, good := quotedName.FindStringSubmatch(r.Message), similarName.FindStringSubmatch(r.Hint)
	if bad == nil || good == nil || !r.Pos.Known() {
		return nil
	}
	line, ok := src.Line(r.Pos.Line)
	if !ok {
		return nil
	}
	// the name is written where the report points, or after it on its line,
	// as in the member of util.pritnln
	col := wordIn(line, bad[1], r.Pos.Column-1)
	if col < 0 {
		col = wordIn(line, bad[1], 0)
	}
	if col < 0 {
		return nil
	}
	edit := TextEdit{
		Range:   Range{toPosition(src, r.Pos.Line, col+1), toPosition(src, r.Pos.Line, col+1+len(bad[1]))},
		NewText: good[1],
	}
	return &Fixes{Fixes: []Fix{{Title: fmt.Sprintf("Change '%s' to '%s'", bad[1], good[1]), Edit: edit}}}
}

// wordIn finds name as a whole word in line, from byte from on
func wordIn(line []byte, name string, from int) int {
	isWord := func(c byte) bool {
		return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
	}
	for i := max(from, 0); i+len(name) <= len(line); i++ {
		if string(line[i:i+len(name)]) != name {
			continue
		}
		if (i == 0 || !isWord(line[i-1])) && (i+len(name) == len(line) || !isWord(line[i+len(name)])) {
			return i
		}
	}
	return -1
}

// message is a report's text, with its label and hint
func message(r *diag.Report) string {
	msg := r.Message
	if r.Label != "" {
		msg += " (" + r.Label + ")"
	}
	if r.Hint != "" {
		msg += "\nhint: " + r.Hint
	}
	return msg
}

// reportsOf finds the reports an error carries: every one of a list of syntax
// or compile errors, or the one of a single error
func reportsOf(err error) []*diag.Report {
	var list interface{ Diagnostics() []*diag.Report }
	if errors.As(err, &list) {
		return list.Diagnostics()
	}
	var one interface{ Diagnostic() *diag.Report }
	if errors.As(err, &one) {
		return []*diag.Report{one.Diagnostic()}
	}
	return nil
}

// tree is the syntax of a file the analysis read; a file with syntax errors
// gives what parsed
func (s *snapshot) tree(file string) ast.Node {
	if t, ok := s.trees[file]; ok {
		return t
	}
	t, _ := parser.ParseSource(s.source(file))
	s.trees[file] = t
	return t
}

// packageOf is the package a file declares and the folder it is in
func (s *snapshot) packageOf(file string) string {
	return fmt.Sprintf("%s\x00%s", path.Dir(file), parser.PeekPackageName(s.source(file)))
}
