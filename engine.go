package evie

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/vm"
)

// Engine is a VM instance together with everything needed to load programs:
// host packages, the standard library and files found through imports. The
// embedded Instance provides Call, Lookup, SetStatic and the lower-level API.
//
// An engine runs one program. Packages are identified by the file or folder
// they come from, and each package name can be loaded once.
type Engine struct {
	*vm.Instance

	opts         Options
	root         string
	packages     map[string]*vm.Package // host packages by name
	dirStack     []string
	filePackages map[string]*vm.Package
	dirPackages  map[string]*vm.Package
	loading      map[string]bool
	cycleTrace   []string
	loadedFiles  []string
	ran          bool
}

// errRanAlready is returned when an engine is asked to run a second program
var errRanAlready = errors.New("this engine already ran a program; create a new engine for each program")

// LoadedFiles returns every file that has been evaluated, in order
func (e *Engine) LoadedFiles() []string {
	return e.loadedFiles
}

// Eval parses and runs a script. A script that declares a package is loaded
// as that package and nil is returned; a bare script returns its result.
func (e *Engine) Eval(script string) (vm.Value, error) {
	return e.EvalSource(diag.NewSource("", []byte(script)))
}

// EvalContext is Eval bound to ctx: when ctx is done, the script stops with an
// error whose cause is ctx's cause, such as context.DeadlineExceeded
func (e *Engine) EvalContext(ctx context.Context, script string) (vm.Value, error) {
	return e.EvalSourceContext(ctx, diag.NewSource("", []byte(script)))
}

// Load compiles a source that declares a package and returns the package;
// name is what errors call the source. Loading a package name a second time
// is an error.
func (e *Engine) Load(name string, src []byte) (*vm.Package, error) {
	return e.EvalPackage(diag.NewSource(name, src))
}

// display names a file the way errors should show it: relative to the working
// directory when it is below it
func display(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return path
}

// Close waits for every task of the engine to finish
func (e *Engine) Close() {
	e.WaitForNoActivity()
}

// resolve finds a package for an import path. A name such as "io" is looked
// up among loaded packages, host packages, the standard library and the
// host's resolver before the filesystem; a path such as "./util" goes to the
// filesystem.
func (e *Engine) resolve(importPath string) (*vm.Package, error) {
	if isPathImport(importPath) {
		return e.resolveFromFilesystem(importPath)
	}
	if pkg := e.Package(importPath); pkg != nil {
		return pkg, nil
	}
	if pkg, ok := e.packages[importPath]; ok {
		e.noteShadowed(importPath, pkg, "host's")
		return pkg, nil
	}
	if constructor, ok := stdlib[importPath]; ok && !e.opts.NoStdlib {
		pkg := constructor()
		e.noteShadowed(importPath, pkg, "standard")
		return pkg, nil
	}
	if e.opts.Resolve != nil {
		pkg, err := e.opts.Resolve(importPath)
		if err != nil {
			return nil, err
		}
		if pkg != nil {
			return pkg, nil
		}
	}
	return e.resolveFromFilesystem(importPath)
}

// noteShadowed tells a script that asks a named package for a member it lacks
// about a local file or folder of the same name, which needs a path import
func (e *Engine) noteShadowed(name string, pkg *vm.Package, kind string) {
	for _, candidate := range e.candidates(name, e.currentDir()) {
		if _, err := os.Stat(candidate); err == nil {
			pkg.SetMemberHint(fmt.Sprintf("imports(\"%s\") is the %s %s package; import \"./%s\" for %s", name, kind, name, name, display(candidate)))
			return
		}
	}
}

func (e *Engine) currentDir() string {
	if len(e.dirStack) > 0 {
		if dir := e.dirStack[len(e.dirStack)-1]; dir != "" {
			return dir
		}
	}
	return e.root
}

func (e *Engine) resolveFromFilesystem(importPath string) (*vm.Package, error) {
	candidates := e.candidates(importPath, e.currentDir())
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.IsDir() {
			return e.loadDirectoryPackage(candidate)
		}
		return e.loadFilePackage(candidate)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("package '%s' not found", importPath)
	}
	tried := make([]string, len(candidates))
	for i, c := range candidates {
		tried[i] = display(c)
	}
	return nil, fmt.Errorf("not found; looked for %s", strings.Join(tried, " and "))
}

func (e *Engine) candidates(importPath, currentDir string) []string {
	var candidates []string

	if filepath.IsAbs(importPath) {
		candidates = append(candidates, importPath)
		if !strings.HasSuffix(importPath, ".ev") {
			candidates = append(candidates, importPath+".ev")
		}
		return candidates
	}

	clean := filepath.Clean(importPath)
	if currentDir != "" {
		candidates = append(candidates, filepath.Join(currentDir, clean+".ev"), filepath.Join(currentDir, clean))
	}
	if e.root != "" && e.root != currentDir {
		candidates = append(candidates, filepath.Join(e.root, clean+".ev"), filepath.Join(e.root, clean))
	}
	return candidates
}

// enter marks a file or directory as being loaded, for cycle detection and
// relative resolution; the returned function undoes it
func (e *Engine) enter(path string, dir string) (func(), error) {
	if e.loading[path] {
		cycle := append(e.cycleTrace, path)
		return nil, fmt.Errorf("cyclic import detected: %s", strings.Join(cycle, " -> "))
	}
	e.loading[path] = true
	e.cycleTrace = append(e.cycleTrace, path)
	e.dirStack = append(e.dirStack, dir)
	return func() {
		delete(e.loading, path)
		e.cycleTrace = e.cycleTrace[:len(e.cycleTrace)-1]
		e.dirStack = e.dirStack[:len(e.dirStack)-1]
	}, nil
}

// readSource reads a file as a source named the way errors show it
func readSource(path string) (*diag.Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", display(path), err)
	}
	return diag.NewSource(display(path), data), nil
}

// packageName parses a source far enough to tell the package it declares;
// a bare script declares none
func packageName(src *diag.Source) (string, error) {
	node, err := parser.ParseSource(src)
	if err != nil {
		return "", err
	}
	if pkg, ok := node.(ast.Package); ok {
		return pkg.Name, nil
	}
	return "", nil
}

// evalPackage compiles sources as one package and records them as loaded. A
// package imported while another loads stops with the importer's context.
func (e *Engine) evalPackage(paths []string, sources []*diag.Source) (*vm.Package, error) {
	pkg, err := e.EvalPackage(sources...)
	if err != nil {
		return nil, err
	}
	e.loadedFiles = append(e.loadedFiles, paths...)
	return pkg, nil
}

// evalMain is evalPackage for the program's main package, bound to ctx
func (e *Engine) evalMain(ctx context.Context, paths []string, sources []*diag.Source) error {
	if _, err := e.EvalPackageContext(ctx, sources...); err != nil {
		return err
	}
	e.loadedFiles = append(e.loadedFiles, paths...)
	return nil
}

func (e *Engine) loadFilePackage(filePath string) (*vm.Package, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}
	if pkg, exists := e.filePackages[absPath]; exists {
		return pkg, nil
	}

	leave, err := e.enter(absPath, filepath.Dir(absPath))
	if err != nil {
		return nil, err
	}
	defer leave()

	src, err := readSource(absPath)
	if err != nil {
		return nil, err
	}
	switch name, err := packageName(src); {
	case err != nil:
		return nil, err
	case name == "":
		return nil, fmt.Errorf("%s does not declare a package", src.Name)
	case name == "main":
		return nil, fmt.Errorf("%s declares package main, which cannot be imported", src.Name)
	}

	pkg, err := e.evalPackage([]string{absPath}, []*diag.Source{src})
	if err != nil {
		return nil, err
	}
	e.filePackages[absPath] = pkg
	return pkg, nil
}

func (e *Engine) loadDirectoryPackage(dirPath string) (*vm.Package, error) {
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, err
	}
	if pkg, exists := e.dirPackages[absDir]; exists {
		return pkg, nil
	}

	leave, err := e.enter(absDir, absDir)
	if err != nil {
		return nil, err
	}
	defer leave()

	files, err := evFiles(absDir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .ev files in %s", display(absDir))
	}
	sources := make([]*diag.Source, len(files))
	for i, file := range files {
		if sources[i], err = readSource(file); err != nil {
			return nil, err
		}
	}

	pkg, err := e.evalPackage(files, sources)
	if err != nil {
		return nil, err
	}
	e.dirPackages[absDir] = pkg
	return pkg, nil
}

// evFiles lists the .ev files of a directory in name order
func evFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", display(dir), err)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ev") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// RunFile evaluates a script file, resolving its imports relative to it, and
// runs main.main when the program defines it. Only that file is loaded; use
// RunProject for a main package spread over several files.
func (e *Engine) RunFile(filePath string) (vm.Value, error) {
	return e.RunFileContext(context.Background(), filePath)
}

// RunFileContext is RunFile bound to ctx: when ctx is done, the program stops
// with an error whose cause is ctx's cause, such as context.DeadlineExceeded.
// That covers loading its packages, main.main and the tasks they start.
func (e *Engine) RunFileContext(ctx context.Context, filePath string) (vm.Value, error) {
	if e.ran {
		return vm.Value{}, errRanAlready
	}
	e.ran = true

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return vm.Value{}, err
	}
	if e.root == "" {
		e.root = filepath.Dir(absPath)
	}
	e.dirStack = append(e.dirStack, filepath.Dir(absPath))
	defer func() { e.dirStack = e.dirStack[:len(e.dirStack)-1] }()

	src, err := readSource(absPath)
	if err != nil {
		return vm.Value{}, err
	}
	name, err := packageName(src)
	if err != nil {
		return vm.Value{}, err
	}
	if name == "" {
		// a bare script runs as it is and its value is the result
		result, err := e.EvalSourceContext(ctx, src)
		if err != nil {
			return vm.Value{}, err
		}
		e.loadedFiles = append(e.loadedFiles, absPath)
		return result, nil
	}

	if err := e.evalMain(ctx, []string{absPath}, []*diag.Source{src}); err != nil {
		return vm.Value{}, err
	}
	return e.runMain(ctx)
}

// RunProject runs the program in a directory. Every file at its top level that
// declares package main is part of the main package; the other files and the
// subfolders are packages the program imports by path.
func (e *Engine) RunProject(dirPath string) (vm.Value, error) {
	return e.RunProjectContext(context.Background(), dirPath)
}

// RunProjectContext is RunProject bound to ctx, the way RunFileContext is
func (e *Engine) RunProjectContext(ctx context.Context, dirPath string) (vm.Value, error) {
	if e.ran {
		return vm.Value{}, errRanAlready
	}
	e.ran = true

	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return vm.Value{}, err
	}
	e.root = absDir
	e.dirStack = append(e.dirStack, absDir)
	defer func() { e.dirStack = e.dirStack[:len(e.dirStack)-1] }()

	files, err := evFiles(absDir)
	if err != nil {
		return vm.Value{}, err
	}
	var paths []string
	var sources []*diag.Source
	for _, file := range files {
		src, err := readSource(file)
		if err != nil {
			return vm.Value{}, err
		}
		name, err := packageName(src)
		if err != nil {
			return vm.Value{}, err
		}
		if name == "main" {
			paths, sources = append(paths, file), append(sources, src)
		}
	}
	if len(sources) == 0 {
		return vm.Value{}, fmt.Errorf("no file in %s declares package main", display(absDir))
	}

	if err := e.evalMain(ctx, paths, sources); err != nil {
		return vm.Value{}, err
	}
	return e.runMain(ctx)
}

// runMain calls main.main when the program defines it and waits for its tasks
func (e *Engine) runMain(ctx context.Context) (vm.Value, error) {
	mainFn, ok := e.Lookup("main.main")
	if !ok || !mainFn.IsCallable() {
		return vm.Value{}, nil
	}
	res, err := e.CallContext(ctx, "main.main")
	e.Close()
	return res, err
}

// isPathImport reports whether an import names a file or folder, such as
// "./util" or "../lib/text", rather than a package by name, such as "io"
func isPathImport(path string) bool {
	return strings.HasPrefix(path, ".") || strings.ContainsAny(path, "/\\") || filepath.IsAbs(path)
}
