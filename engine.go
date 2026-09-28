package evie

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/vm"
)

// Engine is a VM instance together with everything needed to load programs:
// host packages, the standard library and files found through imports. The
// embedded Instance provides Call, Lookup, SetStatic and the lower-level API.
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
}

// LoadedFiles returns every file that has been evaluated, in order
func (e *Engine) LoadedFiles() []string {
	return e.loadedFiles
}

// Eval parses and runs a script. A script that declares a package is loaded
// as that package and nil is returned; a bare script returns its result.
func (e *Engine) Eval(script string) (vm.Value, error) {
	return e.EvalString(script)
}

// Load compiles a source that declares a package and returns the package
func (e *Engine) Load(src []byte) (*vm.Package, error) {
	node, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	pkgNode, isPkg := node.(ast.Package)
	if !isPkg {
		return nil, fmt.Errorf("source does not declare a package")
	}
	if _, err := e.EvalNode(node); err != nil {
		return nil, err
	}
	return e.GetPackage(pkgNode.Name), nil
}

// Close waits for every task of the engine to finish
func (e *Engine) Close() {
	e.WaitForNoActivity()
}

// resolve finds a package for an import path: host packages first, then the
// standard library, then the host's resolver, then the filesystem
func (e *Engine) resolve(importPath string) (*vm.Package, error) {
	if pkg := e.GetPackage(importPath); pkg != nil {
		return pkg, nil
	}
	if pkg, ok := e.packages[importPath]; ok {
		return pkg, nil
	}
	if constructor, ok := stdlib[importPath]; ok && !e.opts.NoStdlib {
		return constructor(), nil
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

func (e *Engine) resolveFromFilesystem(importPath string) (*vm.Package, error) {
	var currentDir string
	if len(e.dirStack) > 0 {
		currentDir = e.dirStack[len(e.dirStack)-1]
	}
	if currentDir == "" {
		currentDir = e.root
	}

	for _, candidate := range e.candidates(importPath, currentDir) {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.IsDir() {
			return e.loadDirectoryPackage(candidate)
		}
		return e.loadFilePackage(candidate)
	}

	return nil, fmt.Errorf("package '%s' not found (searched relative to %s)", importPath, currentDir)
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

// evalFile parses and evaluates one file and returns the package it declared
func (e *Engine) evalFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}
	node, err := parser.Parse(data)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if _, err := e.EvalNode(node); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	e.loadedFiles = append(e.loadedFiles, path)
	if pkgNode, isPkg := node.(ast.Package); isPkg {
		return pkgNode.Name, nil
	}
	return "", nil
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

	name, err := e.evalFile(absPath)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("file %s did not declare a package", absPath)
	}
	pkg := e.GetPackage(name)
	if pkg == nil {
		return nil, fmt.Errorf("package '%s' was not registered after evaluating %s", name, absPath)
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

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", absDir, err)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ev") {
			files = append(files, filepath.Join(absDir, entry.Name()))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .ev files found in directory %s", absDir)
	}
	sort.Strings(files)

	var name string
	for _, file := range files {
		declared, err := e.evalFile(file)
		if err != nil {
			return nil, err
		}
		if declared != "" {
			name = declared
		}
	}
	if name == "" {
		return nil, fmt.Errorf("directory %s contained no package declarations", absDir)
	}
	pkg := e.GetPackage(name)
	if pkg == nil {
		return nil, fmt.Errorf("package '%s' was not registered after evaluating %s", name, absDir)
	}
	e.dirPackages[absDir] = pkg
	return pkg, nil
}

// RunFile evaluates a script file, resolving its imports relative to it, and
// runs main.main when the program defines it
func (e *Engine) RunFile(filePath string) (vm.Value, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return vm.Value{}, err
	}
	if e.root == "" {
		e.root = filepath.Dir(absPath)
	}

	e.dirStack = append(e.dirStack, filepath.Dir(absPath))
	defer func() { e.dirStack = e.dirStack[:len(e.dirStack)-1] }()

	data, err := os.ReadFile(absPath)
	if err != nil {
		return vm.Value{}, fmt.Errorf("failed to read entry file: %w", err)
	}
	node, err := parser.Parse(data)
	if err != nil {
		return vm.Value{}, fmt.Errorf("%s: %w", absPath, err)
	}
	result, err := e.EvalNode(node)
	if err != nil {
		return vm.Value{}, err
	}
	e.loadedFiles = append(e.loadedFiles, absPath)

	if mainFn, ok := e.Lookup("main.main"); ok && mainFn.IsCallable() {
		res, exc := mainFn.Call(nil)
		e.Close()
		if exc != nil {
			return res, exc
		}
		return res, nil
	}
	return result, nil
}

// RunProject evaluates the main.ev of a project directory
func (e *Engine) RunProject(dirPath string) (vm.Value, error) {
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return vm.Value{}, err
	}
	e.root = absDir
	mainFile := filepath.Join(absDir, "main.ev")
	if _, err := os.Stat(mainFile); err != nil {
		return vm.Value{}, fmt.Errorf("main.ev not found in project directory %s", absDir)
	}
	return e.RunFile(mainFile)
}
