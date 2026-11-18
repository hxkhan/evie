package evie

import (
	"context"
	"errors"
	"fmt"
	goio "io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/std/io"
	"github.com/hxkhan/evie/vm"
)

// Engine is a VM instance together with everything needed to load programs:
// host packages, the standard library, and files or Bundles found through
// imports.
//
// An engine runs one program. Packages are identified by the file or folder
// they come from, and each package name can be loaded once.
type Engine struct {
	vm *vm.Instance

	opts         Options
	root         string
	stdAllow     map[string]bool        // when non-nil, restricts stdlib imports to these names
	packages     map[string]*vm.Package // host packages by name (cloned per engine)
	bundleLoaded map[string]*vm.Package // canonical key -> compiled package for path-imported modules
	curBundlePkg *parsedPackage         // package currently being compiled during LoadBundle
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

// Close waits for every task of the engine to finish
func (e *Engine) Close() {
	e.vm.WaitForNoActivity()
}

// Dump writes a human-readable summary of every compiled package, global,
// class, function, local stack slot, escape decision, and closure capture in
// the engine. Set Options.Debug.DebugInfo before loading to include slot names,
// local classes, nested closures, and resolved capture names.
func (e *Engine) Dump(w goio.Writer) {
	e.vm.Dump(w)
}

func (e *Engine) sfs() sourceFS {
	if e.opts.FS != nil {
		return virtualSourceFS{fsys: e.opts.FS}
	}
	return osSourceFS{}
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

// resolve finds a package for an import path. During bundle loading, path
// imports resolve directly from already-compiled packages in O(1) without
// touching the filesystem or re-entering the VM. Bare names resolve via loaded
// packages, host packages, the standard library, and Options.Resolve.
func (e *Engine) resolve(importPath string) (*vm.Package, error) {
	if e.curBundlePkg != nil {
		if targetKey, ok := e.curBundlePkg.deps[importPath]; ok {
			if pkg, loaded := e.bundleLoaded[targetKey]; loaded {
				return pkg, nil
			}
		}
	}

	if isPathImport(importPath) {
		return e.resolvePathAdHoc(importPath)
	}
	if pkg, ok := e.packages[importPath]; ok {
		e.noteShadowed(importPath, pkg, "host's")
		return pkg, nil
	}
	if constructor, ok := stdlib[importPath]; ok && !e.opts.NoStdlib && (e.stdAllow == nil || e.stdAllow[importPath]) {
		stdID := "std:" + importPath
		if pkg := e.vm.PackageByID(stdID); pkg != nil {
			e.noteShadowed(importPath, pkg, "standard")
			return pkg, nil
		}
		var pkg *vm.Package
		if importPath == "io" && (e.opts.Stdin != nil || e.opts.Stdout != nil) {
			pkg = io.ConstructWith(e.opts.Stdin, e.opts.Stdout)
		} else {
			pkg = constructor()
		}
		pkg.SetID(stdID)
		e.noteShadowed(importPath, pkg, "standard")
		return pkg, nil
	}
	if pkg := e.vm.PackageByID(importPath); pkg != nil {
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
	return nil, fmt.Errorf("package '%s' not found", importPath)
}

// resolvePathAdHoc handles relative path imports coming from inline Eval/Load
// calls (outside a pre-parsed Bundle), respecting Options.FS and Options.Root.
func (e *Engine) resolvePathAdHoc(importPath string) (*vm.Package, error) {
	sfs := e.sfs()
	root := e.root
	if root == "" && e.opts.FS != nil {
		root = "."
	}
	loader := newGraphLoader(sfs, root)
	dep, err := loader.resolvePath(importPath, root, importSite{importPath: importPath})
	if err != nil {
		return nil, err
	}
	for _, p := range loader.order {
		if _, loaded := e.bundleLoaded[p.key]; loaded {
			continue
		}
		if err := e.compileParsedPackage(context.Background(), p); err != nil {
			return nil, err
		}
	}
	return e.bundleLoaded[dep.key], nil
}

// noteShadowed tells a script that asks a named package for a member it lacks
// about a local file or folder of the same name, which needs a path import
func (e *Engine) noteShadowed(name string, pkg *vm.Package, kind string) {
	if e.curBundlePkg != nil {
		if localPath, ok := e.curBundlePkg.shadowed[name]; ok {
			pkg.SetMemberHint(fmt.Sprintf("imports(\"%s\") is the %s %s package; import \"./%s\" for %s", name, kind, name, name, localPath))
		}
		return
	}
	if e.root == "" && e.opts.FS == nil {
		return
	}
	sfs := e.sfs()
	dir := e.root
	if dir == "" {
		dir = "."
	}
	cands, err := sfs.candidates(name, dir, "")
	if err != nil {
		return
	}
	for _, cand := range cands {
		if _, exists := sfs.stat(cand); exists {
			pkg.SetMemberHint(fmt.Sprintf("imports(\"%s\") is the %s %s package; import \"./%s\" for %s", name, kind, name, name, sfs.display(cand)))
			return
		}
	}
}

func (e *Engine) compileParsedPackage(ctx context.Context, p *parsedPackage) error {
	vmUnits := make([]vm.ParsedUnit, len(p.units))
	paths := make([]string, len(p.units))
	for i, u := range p.units {
		vmUnits[i] = vm.ParsedUnit{Source: u.src, Node: u.node}
		paths[i] = u.path
	}

	prevPkg := e.curBundlePkg
	e.curBundlePkg = p
	pkg, err := e.vm.EvalParsedPackageContext(ctx, p.key, vmUnits)
	e.curBundlePkg = prevPkg
	if err != nil {
		if p.importedBy != nil {
			return attachImportNote(err, *p.importedBy)
		}
		return err
	}
	e.bundleLoaded[p.key] = pkg
	e.loadedFiles = append(e.loadedFiles, paths...)
	return nil
}

// LoadBundle compiles and loads a pre-parsed Bundle into the engine in
// topological dependency order without invoking main.main (and without
// executing a bare script).
func (e *Engine) LoadBundle(b *Bundle) error {
	return e.LoadBundleContext(context.Background(), b)
}

// LoadBundleContext is LoadBundle bound to ctx.
func (e *Engine) LoadBundleContext(ctx context.Context, b *Bundle) error {
	if b == nil {
		return errors.New("cannot load a nil Bundle")
	}
	if e.ran {
		return errRanAlready
	}
	e.ran = true
	if e.root == "" {
		e.root = b.root
	}

	if b.script != nil {
		if err := e.vm.CompileParsedSource(vm.ParsedUnit{Source: b.script.src, Node: b.script.node}); err != nil {
			return err
		}
		e.loadedFiles = append(e.loadedFiles, b.script.path)
		return nil
	}

	for _, p := range b.packages {
		if err := e.compileParsedPackage(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// RunBundle compiles, loads, and executes a pre-parsed Bundle without touching
// the filesystem or re-parsing any source files.
func (e *Engine) RunBundle(b *Bundle) (vm.Value, error) {
	return e.RunBundleContext(context.Background(), b)
}

// RunBundleContext is RunBundle bound to ctx: when ctx is done, package
// initializers, main.main, and any tasks they start stop with ctx's cause.
func (e *Engine) RunBundleContext(ctx context.Context, b *Bundle) (vm.Value, error) {
	if b == nil {
		return vm.Value{}, errors.New("cannot run a nil Bundle")
	}
	if e.ran {
		return vm.Value{}, errRanAlready
	}
	e.ran = true
	if e.root == "" {
		e.root = b.root
	}

	if b.script != nil {
		result, err := e.vm.EvalParsedSourceContext(ctx, vm.ParsedUnit{Source: b.script.src, Node: b.script.node})
		if ctx.Err() == nil {
			e.Close()
		}
		if err != nil {
			return vm.Value{}, err
		}
		e.loadedFiles = append(e.loadedFiles, b.script.path)
		return result, nil
	}

	for _, p := range b.packages {
		if err := e.compileParsedPackage(ctx, p); err != nil {
			return vm.Value{}, err
		}
	}
	return e.runMain(ctx)
}

// RunFile evaluates a script file, resolving its imports relative to it (or
// inside Options.FS when configured), and runs main.main when defined.
func (e *Engine) RunFile(filePath string) (vm.Value, error) {
	return e.RunFileContext(context.Background(), filePath)
}

// RunFileContext is RunFile bound to ctx: when ctx is done, the program stops
// with an error whose cause is ctx's cause, such as context.DeadlineExceeded.
func (e *Engine) RunFileContext(ctx context.Context, filePath string) (vm.Value, error) {
	if e.ran {
		return vm.Value{}, errRanAlready
	}
	bundle, err := parseFile(e.sfs(), filePath, e.root, e.opts.FS)
	if err != nil {
		return vm.Value{}, err
	}
	return e.RunBundleContext(ctx, bundle)
}

// LoadFile compiles and loads a single file and all of its imports without
// invoking main.main. For bare scripts without a package header, the script is
// compiled without executing it.
func (e *Engine) LoadFile(filePath string) error {
	if e.ran {
		return errRanAlready
	}
	bundle, err := parseFile(e.sfs(), filePath, e.root, e.opts.FS)
	if err != nil {
		return err
	}
	return e.LoadBundle(bundle)
}

// RunProject runs the program in a directory (on disk or inside Options.FS).
// Every file at its top level that declares package main is part of the main
// package; the other files and subfolders are packages imported by path.
func (e *Engine) RunProject(dirPath string) (vm.Value, error) {
	return e.RunProjectContext(context.Background(), dirPath)
}

// RunProjectContext is RunProject bound to ctx, the way RunFileContext is.
func (e *Engine) RunProjectContext(ctx context.Context, dirPath string) (vm.Value, error) {
	if e.ran {
		return vm.Value{}, errRanAlready
	}
	bundle, err := parseProject(e.sfs(), dirPath, e.opts.FS)
	if err != nil {
		return vm.Value{}, err
	}
	return e.RunBundleContext(ctx, bundle)
}

// LoadProject compiles and loads the program in a directory without invoking
// main.main.
func (e *Engine) LoadProject(dirPath string) error {
	if e.ran {
		return errRanAlready
	}
	bundle, err := parseProject(e.sfs(), dirPath, e.opts.FS)
	if err != nil {
		return err
	}
	return e.LoadBundle(bundle)
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
func isPathImport(p string) bool {
	return strings.HasPrefix(p, ".") || strings.ContainsAny(p, "/\\") || filepath.IsAbs(p)
}
