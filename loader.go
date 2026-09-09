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

// LoaderOptions configures a Loader instance.
type LoaderOptions struct {
	RootDir         string             // project root directory
	ImportsResolver vm.PackageResolver // custom host package resolver
	Inline          bool               // enable instruction inlining optimization
	Metrics         bool               // collect execution metrics
	LogCaptures     bool               // log captured variables
	LogCache        bool               // log cache hits/misses
}

// Loader manages package resolution, multi-file program loading, and script execution.
type Loader struct {
	opts         LoaderOptions
	vm           *vm.Instance
	rootDir      string
	dirStack     []string
	filePackages map[string]vm.Package
	dirPackages  map[string]vm.Package
	loading      map[string]bool
	cycleTrace   []string
	loadedFiles  []string
}

// NewLoader creates a new Loader instance with the given options.
func NewLoader(opts ...LoaderOptions) *Loader {
	var opt LoaderOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	l := &Loader{
		opts:         opt,
		rootDir:      opt.RootDir,
		dirStack:     make([]string, 0, 8),
		filePackages: make(map[string]vm.Package),
		dirPackages:  make(map[string]vm.Package),
		loading:      make(map[string]bool),
		cycleTrace:   make([]string, 0, 8),
		loadedFiles:  make([]string, 0, 16),
	}

	vmOpts := vm.Options{
		LogCache:        opt.LogCache,
		LogCaptures:     opt.LogCaptures,
		DisableInlining: !opt.Inline,
		Metrics:         opt.Metrics,
		TopLevelLogic:   true,
		PackageResolver: l.Resolve,
	}

	l.vm = vm.New(vmOpts)
	return l
}

// VM returns the underlying virtual machine instance.
func (l *Loader) VM() *vm.Instance {
	return l.vm
}

// LoadedFiles returns the list of all file paths that have been evaluated.
func (l *Loader) LoadedFiles() []string {
	return l.loadedFiles
}

// Resolve resolves an imported package name or path.
func (l *Loader) Resolve(importPath string) (vm.Package, error) {
	// 1. Check if already registered by name in the VM
	if pkg := l.vm.GetPackage(importPath); pkg != nil {
		return pkg, nil
	}

	// 2. Custom host resolver (if provided)
	if l.opts.ImportsResolver != nil {
		if pkg, err := l.opts.ImportsResolver(importPath); err == nil && pkg != nil {
			return pkg, nil
		}
	}

	// 3. Standard library constructors
	if constructor, exists := StandardLibraryConstructors[importPath]; exists {
		pkg := constructor()
		if pkg.Name() == "" {
			pkg.SetName(importPath)
		}
		return pkg, nil
	}

	// 4. Filesystem resolution
	return l.resolveFromFilesystem(importPath)
}

func (l *Loader) resolveFromFilesystem(importPath string) (vm.Package, error) {
	var currentDir string
	if len(l.dirStack) > 0 {
		currentDir = l.dirStack[len(l.dirStack)-1]
	}
	if currentDir == "" {
		currentDir = l.rootDir
	}

	candidates := l.getCandidates(importPath, currentDir)
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}

		if info.IsDir() {
			return l.loadDirectoryPackage(candidate)
		}
		return l.loadFilePackage(candidate)
	}

	return nil, fmt.Errorf("package '%s' not found (searched relative to %s)", importPath, currentDir)
}

func (l *Loader) getCandidates(importPath, currentDir string) []string {
	var candidates []string

	if filepath.IsAbs(importPath) {
		candidates = append(candidates, importPath)
		if !strings.HasSuffix(importPath, ".ev") {
			candidates = append(candidates, importPath+".ev")
		}
		return candidates
	}

	clean := filepath.Clean(importPath)

	// Check relative to current directory
	if currentDir != "" {
		candidates = append(candidates, filepath.Join(currentDir, clean+".ev"))
		candidates = append(candidates, filepath.Join(currentDir, clean))
	}

	// Check relative to project root
	if l.rootDir != "" && l.rootDir != currentDir {
		candidates = append(candidates, filepath.Join(l.rootDir, clean+".ev"))
		candidates = append(candidates, filepath.Join(l.rootDir, clean))
	}

	return candidates
}

func (l *Loader) loadFilePackage(filePath string) (vm.Package, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, err
	}

	if pkg, exists := l.filePackages[absPath]; exists {
		return pkg, nil
	}

	if l.loading[absPath] {
		cycle := append(l.cycleTrace, absPath)
		return nil, fmt.Errorf("cyclic import detected: %s", strings.Join(cycle, " -> "))
	}

	l.loading[absPath] = true
	l.cycleTrace = append(l.cycleTrace, absPath)
	l.dirStack = append(l.dirStack, filepath.Dir(absPath))
	defer func() {
		delete(l.loading, absPath)
		l.cycleTrace = l.cycleTrace[:len(l.cycleTrace)-1]
		l.dirStack = l.dirStack[:len(l.dirStack)-1]
	}()

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", absPath, err)
	}

	node, err := parser.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("syntax error in %s: %w", absPath, err)
	}

	_, err = l.vm.EvalNode(node)
	if err != nil {
		return nil, fmt.Errorf("evaluation error in %s: %w", absPath, err)
	}

	l.loadedFiles = append(l.loadedFiles, absPath)

	pkgNode, isPkg := node.(ast.Package)
	if !isPkg {
		return nil, fmt.Errorf("file %s did not declare a package", absPath)
	}

	pkg := l.vm.GetPackage(pkgNode.Name)
	if pkg == nil {
		return nil, fmt.Errorf("package '%s' was not registered after evaluating %s", pkgNode.Name, absPath)
	}

	l.filePackages[absPath] = pkg
	return pkg, nil
}

func (l *Loader) loadDirectoryPackage(dirPath string) (vm.Package, error) {
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, err
	}

	if pkg, exists := l.dirPackages[absDir]; exists {
		return pkg, nil
	}

	if l.loading[absDir] {
		cycle := append(l.cycleTrace, absDir)
		return nil, fmt.Errorf("cyclic import detected: %s", strings.Join(cycle, " -> "))
	}

	l.loading[absDir] = true
	l.cycleTrace = append(l.cycleTrace, absDir)
	l.dirStack = append(l.dirStack, absDir)
	defer func() {
		delete(l.loading, absDir)
		l.cycleTrace = l.cycleTrace[:len(l.cycleTrace)-1]
		l.dirStack = l.dirStack[:len(l.dirStack)-1]
	}()

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", absDir, err)
	}

	var evFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ev") {
			evFiles = append(evFiles, filepath.Join(absDir, entry.Name()))
		}
	}

	if len(evFiles) == 0 {
		return nil, fmt.Errorf("no .ev files found in directory %s", absDir)
	}

	sort.Strings(evFiles)

	var lastPkgName string
	for _, filePath := range evFiles {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", filePath, err)
		}

		node, err := parser.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("syntax error in %s: %w", filePath, err)
		}

		if pkgNode, isPkg := node.(ast.Package); isPkg {
			lastPkgName = pkgNode.Name
		}

		_, err = l.vm.EvalNode(node)
		if err != nil {
			return nil, fmt.Errorf("evaluation error in %s: %w", filePath, err)
		}

		l.loadedFiles = append(l.loadedFiles, filePath)
	}

	if lastPkgName == "" {
		return nil, fmt.Errorf("directory %s contained no package declarations", absDir)
	}

	pkg := l.vm.GetPackage(lastPkgName)
	if pkg == nil {
		return nil, fmt.Errorf("package '%s' was not registered after evaluating %s", lastPkgName, absDir)
	}

	l.dirPackages[absDir] = pkg
	return pkg, nil
}

// RunFile executes an Evie script file as the main entry point.
// If the file or any package defines a main.main() function, it automatically invokes it.
func (l *Loader) RunFile(filePath string) (vm.Value, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return vm.Value{}, err
	}

	if l.rootDir == "" {
		l.rootDir = filepath.Dir(absPath)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return vm.Value{}, fmt.Errorf("failed to read entry file: %w", err)
	}

	l.dirStack = append(l.dirStack, filepath.Dir(absPath))
	defer func() {
		l.dirStack = l.dirStack[:len(l.dirStack)-1]
	}()

	node, err := parser.Parse(data)
	if err != nil {
		return vm.Value{}, fmt.Errorf("syntax error in %s: %w", absPath, err)
	}

	evalResult, err := l.vm.EvalNode(node)
	if err != nil {
		return vm.Value{}, err
	}
	l.loadedFiles = append(l.loadedFiles, absPath)

	// If main.main() exists and is callable, invoke it
	mainFn, ok := l.vm.Lookup("main.main")
	if ok && mainFn.IsCallable() {
		fbr := l.vm.NewFiber()
		res, exc := mainFn.Call(fbr)
		l.vm.WaitForNoActivity()
		if exc != nil {
			return res, exc
		}
		return res, nil
	}

	return evalResult, nil
}

// RunProject executes an Evie project directory, looking for main.ev.
func (l *Loader) RunProject(dirPath string) (vm.Value, error) {
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return vm.Value{}, err
	}

	l.rootDir = absDir
	mainFile := filepath.Join(absDir, "main.ev")
	if _, err := os.Stat(mainFile); err != nil {
		return vm.Value{}, fmt.Errorf("main.ev not found in project directory %s", absDir)
	}

	return l.RunFile(mainFile)
}

// RunFile executes an Evie script file using a default Loader and runs main() if present.
func RunFile(filePath string) (vm.Value, error) {
	return NewLoader(LoaderOptions{Inline: true}).RunFile(filePath)
}

// RunProject executes an Evie project directory (looking for main.ev) using a default Loader.
func RunProject(dirPath string) (vm.Value, error) {
	return NewLoader(LoaderOptions{Inline: true}).RunProject(dirPath)
}
