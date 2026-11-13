package evie

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"sort"
	"strings"

	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/std/io"
	"github.com/hxkhan/evie/vm"
)

// Engine is a VM instance together with everything needed to load programs:
// host packages, the standard library and files found through imports.
//
// An engine runs one program. Packages are identified by the file or folder
// they come from, and each package name can be loaded once.
type Engine struct {
	vm *vm.Instance

	opts         Options
	root         string
	stdAllow     map[string]bool        // when non-nil, restricts stdlib imports to these names
	packages     map[string]*vm.Package // host packages by name (cloned per engine)
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

// LoadedFiles returns a copy of every file path that has been evaluated, in order
func (e *Engine) LoadedFiles() []string {
	return slices.Clone(e.loadedFiles)
}

// Call looks up a callable by path (such as "main.add"), boxes the Go arguments
// with BoxAny and invokes it on a borrowed fiber.
func (e *Engine) Call(path string, args ...any) (vm.Value, error) {
	return e.vm.Call(path, args...)
}

// CallContext is Call bound to ctx: when ctx is done, the call and any tasks it
// starts stop with ctx's cause.
func (e *Engine) CallContext(ctx context.Context, path string, args ...any) (vm.Value, error) {
	return e.vm.CallContext(ctx, path, args...)
}

// Lookup resolves a symbol from a loaded package ("pkg.sym") or main/prelude ("sym").
func (e *Engine) Lookup(path string) (vm.Value, bool) {
	return e.vm.Lookup(path)
}

// SetStatic binds a symbol in the engine's prelude, visible to every package without an import.
func (e *Engine) SetStatic(name string, val vm.Value) {
	e.vm.SetStatic(name, val)
}

// Interrupt stops all code currently running or waiting in this engine.
func (e *Engine) Interrupt() {
	e.vm.Interrupt()
}

// Eval parses and runs a script. A script that declares a package is loaded
// as that package and nil is returned; a bare script returns its result.
func (e *Engine) Eval(script string) (vm.Value, error) {
	return e.vm.EvalSource(diag.NewSource("", []byte(script)))
}

// EvalContext is Eval bound to ctx: when ctx is done, the script stops with an
// error whose cause is ctx's cause, such as context.DeadlineExceeded
func (e *Engine) EvalContext(ctx context.Context, script string) (vm.Value, error) {
	return e.vm.EvalSourceContext(ctx, diag.NewSource("", []byte(script)))
}

// Load compiles a source that declares a package and returns the package;
// name is what errors call the source. Loading a package name a second time
// is an error.
func (e *Engine) Load(name string, src []byte) (*vm.Package, error) {
	return e.vm.EvalPackage(diag.NewSource(name, src))
}

// LoadContext is Load bound to ctx: when ctx is done, package initializers
// stop with ctx's cause.
func (e *Engine) LoadContext(ctx context.Context, name string, src []byte) (*vm.Package, error) {
	return e.vm.EvalPackageContext(ctx, diag.NewSource(name, src))
}

// display names a file the way errors should show it: relative to the working
// directory when it is below it
func display(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
	}
	return path
}

// Close waits for every task of the engine to finish
func (e *Engine) Close() {
	e.vm.WaitForNoActivity()
}

// resolve finds a package for an import path. A name such as "io" is looked
// up among loaded packages, host packages, the standard library and the
// host's resolver before the filesystem; a path such as "./util" goes to the
// filesystem.
func (e *Engine) resolve(importPath string) (*vm.Package, error) {
	if isPathImport(importPath) {
		return e.resolveFromFilesystem(importPath)
	}
	if pkg := e.vm.Package(importPath); pkg != nil {
		return pkg, nil
	}
	if pkg, ok := e.packages[importPath]; ok {
		e.noteShadowed(importPath, pkg, "host's")
		return pkg, nil
	}
	if constructor, ok := stdlib[importPath]; ok && !e.opts.NoStdlib && (e.stdAllow == nil || e.stdAllow[importPath]) {
		var pkg *vm.Package
		if importPath == "io" && (e.opts.Stdin != nil || e.opts.Stdout != nil) {
			pkg = io.ConstructWith(e.opts.Stdin, e.opts.Stdout)
		} else {
			pkg = constructor()
		}
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

// canonicalKey normalizes a path for cache and cycle-detection maps so symlinks
// and Windows case differences refer to the same package.
func canonicalKey(absPath string) string {
	key := absPath
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		key = resolved
	}
	if goruntime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

func isDotRelative(path string) bool {
	return path == "." || path == ".." ||
		strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") ||
		strings.HasPrefix(path, `.\`) || strings.HasPrefix(path, `..\`)
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
	add := func(dir string) {
		if strings.HasSuffix(clean, ".ev") {
			candidates = append(candidates, filepath.Join(dir, clean))
		} else {
			candidates = append(candidates, filepath.Join(dir, clean+".ev"), filepath.Join(dir, clean))
		}
	}
	if currentDir != "" {
		add(currentDir)
	}
	if !isDotRelative(importPath) && e.root != "" && e.root != currentDir {
		add(e.root)
	}
	return candidates
}

// enter marks a file or directory as being loaded, for cycle detection and
// relative resolution; the returned function undoes it
func (e *Engine) enter(key, label, dir string) (func(), error) {
	if e.loading[key] {
		cycle := append(e.cycleTrace, label)
		return nil, fmt.Errorf("cyclic import detected: %s", strings.Join(cycle, " -> "))
	}
	e.loading[key] = true
	e.cycleTrace = append(e.cycleTrace, label)
	e.dirStack = append(e.dirStack, dir)
	return func() {
		delete(e.loading, key)
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

// evalPackage compiles sources as one package and records them as loaded. A
// package imported while another loads stops with the importer's context.
func (e *Engine) evalPackage(paths []string, sources []*diag.Source) (*vm.Package, error) {
	pkg, err := e.vm.EvalPackage(sources...)
	if err != nil {
		return nil, err
	}
	e.loadedFiles = append(e.loadedFiles, paths...)
	return pkg, nil
}

// evalMain is evalPackage for the program's main package, bound to ctx
func (e *Engine) evalMain(ctx context.Context, paths []string, sources []*diag.Source) error {
	if _, err := e.vm.EvalPackageContext(ctx, sources...); err != nil {
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
	key := canonicalKey(absPath)
	if pkg, exists := e.filePackages[key]; exists {
		return pkg, nil
	}

	leave, err := e.enter(key, absPath, filepath.Dir(absPath))
	if err != nil {
		return nil, err
	}
	defer leave()

	src, err := readSource(absPath)
	if err != nil {
		return nil, err
	}
	if parser.PeekPackageName(src) == "main" {
		return nil, fmt.Errorf("%s declares package main, which cannot be imported", src.Name)
	}

	pkg, err := e.evalPackage([]string{absPath}, []*diag.Source{src})
	if err != nil {
		return nil, err
	}
	e.filePackages[key] = pkg
	return pkg, nil
}

func (e *Engine) loadDirectoryPackage(dirPath string) (*vm.Package, error) {
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, err
	}
	dirKey := canonicalKey(absDir)
	if pkg, exists := e.dirPackages[dirKey]; exists {
		return pkg, nil
	}

	leave, err := e.enter(dirKey, absDir, absDir)
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

	fileKeys := make([]string, len(files))
	sources := make([]*diag.Source, len(files))
	for i, file := range files {
		fileKeys[i] = canonicalKey(file)
		if e.loading[fileKeys[i]] {
			cycle := append(e.cycleTrace, file)
			return nil, fmt.Errorf("cyclic import detected: %s", strings.Join(cycle, " -> "))
		}
		if sources[i], err = readSource(file); err != nil {
			return nil, err
		}
		if parser.PeekPackageName(sources[i]) == "main" {
			return nil, fmt.Errorf("%s declares package main, which cannot be imported", sources[i].Name)
		}
	}
	for _, fk := range fileKeys {
		e.loading[fk] = true
	}
	defer func() {
		for _, fk := range fileKeys {
			delete(e.loading, fk)
		}
	}()

	pkg, err := e.evalPackage(files, sources)
	if err != nil {
		return nil, err
	}
	e.dirPackages[dirKey] = pkg
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
	leave, err := e.enter(canonicalKey(absPath), absPath, filepath.Dir(absPath))
	if err != nil {
		return vm.Value{}, err
	}
	defer leave()

	src, err := readSource(absPath)
	if err != nil {
		return vm.Value{}, err
	}
	if parser.PeekPackageName(src) == "" {
		// a bare script runs as it is and its value is the result
		result, err := e.vm.EvalSourceContext(ctx, src)
		if ctx.Err() == nil {
			e.Close()
		}
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
		if parser.PeekPackageName(src) == "main" {
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
	var args []any
	if len(e.opts.Args) > 0 && mainFn.AcceptsArgs() {
		args = make([]any, len(e.opts.Args))
		for i, arg := range e.opts.Args {
			args[i] = arg
		}
	}
	res, err := e.CallContext(ctx, "main.main", args...)
	if ctx.Err() == nil {
		e.Close()
	}
	return res, err
}

// isPathImport reports whether an import names a file or folder, such as
// "./util" or "../lib/text", rather than a package by name, such as "io"
func isPathImport(path string) bool {
	return strings.HasPrefix(path, ".") || strings.ContainsAny(path, "/\\") || filepath.IsAbs(path)
}
