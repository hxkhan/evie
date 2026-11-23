package analysis

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
)

// ---------------------------------------------------------------- symbols of the workspace

// maxSymbols is how many symbols a search of the workspace gives at most
const maxSymbols = 1000

// SymbolInformation is a function, class, method or binding found by a search
// of the whole workspace
type SymbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

// WorkspaceSymbols finds the declarations of every file of the workspace
// whose names hold the letters of query in order, ignoring case: the
// functions, classes and top-level bindings, and the fields and methods of
// classes. Files are only parsed, so this is quick and works on broken code.
func (w *Workspace) WorkspaceSymbols(query string) []SymbolInformation {
	found := []SymbolInformation{}
	for _, file := range evFiles(w.fsys) {
		text, err := fs.ReadFile(w.fsys, file)
		if err != nil {
			continue
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
		var add func(sym Symbol, container string)
		add = func(sym Symbol, container string) {
			if len(found) < maxSymbols && matches(sym.Name, query) {
				found = append(found, SymbolInformation{
					Name:          sym.Name,
					Kind:          sym.Kind,
					Location:      Location{Path: file, Range: sym.SelectionRange},
					ContainerName: container,
				})
			}
			for _, child := range sym.Children {
				add(child, sym.Name)
			}
		}
		for _, stmt := range code {
			if sym, ok := symbolOf(src, stmt, ""); ok {
				add(sym, "")
			}
		}
	}
	return found
}

// matches reports whether the letters of query appear in name in order,
// ignoring case, the way editors match what is typed to a symbol
func matches(name, query string) bool {
	name, query = strings.ToLower(name), strings.ToLower(query)
	for _, c := range query {
		i := strings.IndexRune(name, c)
		if i < 0 {
			return false
		}
		name = name[i+1:]
	}
	return true
}

// ---------------------------------------------------------------- links

// DocumentLink makes a span of a file a link to another file, as an import
// path is to the file or folder it imports
type DocumentLink struct {
	Range   Range  `json:"range"`
	Target  string `json:"target"` // a path of the workspace
	Tooltip string `json:"tooltip,omitempty"`
}

// Links makes the import paths of a file links to what they import: the file,
// or for a folder its first file. A path of the standard library or of the
// host is not a link.
func (w *Workspace) Links(file string) []DocumentLink {
	text, err := fs.ReadFile(w.fsys, file)
	if err != nil {
		return nil
	}
	src := diag.NewSource(file, text)
	node, _ := parser.ParseSource(src)
	pkg, ok := node.(ast.Package)
	if !ok {
		return []DocumentLink{}
	}
	links := []DocumentLink{}
	for _, spec := range pkg.Imports {
		if !spec.Pos.Known() {
			continue
		}
		target, folder, ok := w.resolveImport(path.Dir(file), spec.Path)
		if !ok {
			continue
		}
		tip := "Open " + target
		if folder != "" {
			tip = fmt.Sprintf("Open package %s (%s)", path.Base(folder), target)
		}
		links = append(links, DocumentLink{Range: toRange(src, spec.Pos), Target: target, Tooltip: tip})
	}
	return links
}

// resolveImport finds the file an import path names from dir the way the
// loader does, from dir and then, unless it starts with ./ or ../, from the
// root; for a folder it gives the folder and its first .ev file
func (w *Workspace) resolveImport(dir, importPath string) (file, folder string, ok bool) {
	slash := strings.ReplaceAll(importPath, "\\", "/")
	if slash == "" || strings.HasPrefix(slash, "/") {
		return "", "", false
	}
	bases := []string{dir}
	if !strings.HasPrefix(slash, "./") && !strings.HasPrefix(slash, "../") && dir != "." {
		bases = append(bases, ".")
	}
	for _, base := range bases {
		joined := path.Clean(path.Join(base, slash))
		if joined == ".." || strings.HasPrefix(joined, "../") {
			return "", "", false
		}
		candidates := []string{joined}
		if !strings.HasSuffix(joined, ".ev") {
			candidates = []string{joined + ".ev", joined}
		}
		for _, c := range candidates {
			info, err := fs.Stat(w.fsys, c)
			if err != nil {
				continue
			}
			if !info.IsDir() {
				return c, "", true
			}
			entries, err := fs.ReadDir(w.fsys, c)
			if err != nil {
				continue
			}
			names := []string{}
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".ev") {
					names = append(names, e.Name())
				}
			}
			if len(names) == 0 {
				continue
			}
			slices.Sort(names)
			return path.Join(c, names[0]), c, true
		}
	}
	return "", "", false
}

// ---------------------------------------------------------------- running

// Runnable is a program a file starts: where to offer to run it, and what to
// give evie to run it
type Runnable struct {
	Range   Range  `json:"range"`   // the name of fn main, or the start of a script
	Target  string `json:"target"`  // the file, or for a project its folder
	Project bool   `json:"project"` // the target is a folder of main files
}

// Runnable tells whether file starts a program: a script, or a main file
// that declares fn main. A main file that is part of a project runs the
// project's folder, as evie does with a folder.
func (w *Workspace) Runnable(file string) (Runnable, bool) {
	text, err := fs.ReadFile(w.fsys, file)
	if err != nil {
		return Runnable{}, false
	}
	src := diag.NewSource(file, text)
	switch parser.PeekPackageName(src) {
	case "":
		return Runnable{Target: file}, true
	case "main":
	default:
		return Runnable{}, false
	}
	node, _ := parser.ParseSource(src)
	pkg, ok := node.(ast.Package)
	if !ok {
		return Runnable{}, false
	}
	for _, stmt := range pkg.Code {
		if fn, ok := stmt.(ast.Fn); ok && fn.Name == "main" && !fn.UsedAsExpr && fn.NamePos.Known() {
			r := Runnable{Range: toRange(src, fn.NamePos), Target: file}
			if dir := path.Dir(file); !separatePrograms(w.fsys, dir) {
				r.Target, r.Project = dir, true
			}
			return r, true
		}
	}
	return Runnable{}, false
}
