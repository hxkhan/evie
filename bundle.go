package evie

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"sort"
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/token"
)

// Bundle is a parsed, import-resolved Evie program ready to be loaded or run
// in one or more independent Engines without touching the filesystem or
// re-parsing source files. A Bundle is immutable and safe to share across
// goroutines.
type Bundle struct {
	script   *parsedUnit      // non-nil for a bare script without a package header
	packages []*parsedPackage // packages in topological order (dependencies first, entry last)
	root     string
	fsys     iofs.FS
}

type parsedUnit struct {
	path string
	src  *diag.Source
	node ast.Node
}

type importSite struct {
	src        *diag.Source
	pos        token.Pos
	importPath string
}

type parsedPackage struct {
	key        string
	name       string
	origin     string
	dir        string
	units      []parsedUnit
	deps       map[string]string // import path as written -> target parsedPackage.key
	shadowed   map[string]string // bare import name -> local file/dir display path (for hints)
	importedBy *importSite       // first importer's site, used to annotate compile/init errors
}

// Files returns a copy of every source file path in the bundle, in load order.
func (b *Bundle) Files() []string {
	if b == nil {
		return nil
	}
	if b.script != nil {
		return []string{b.script.path}
	}
	var out []string
	for _, pkg := range b.packages {
		for _, u := range pkg.units {
			out = append(out, u.path)
		}
	}
	return out
}

// Packages returns a copy of the declared package names in topological load order.
func (b *Bundle) Packages() []string {
	if b == nil || len(b.packages) == 0 {
		return nil
	}
	out := make([]string, len(b.packages))
	for i, pkg := range b.packages {
		out[i] = pkg.name
	}
	return out
}

// ParseFile parses a single Evie file and every package it imports by path,
// without running any code. The file and its imports are read from fsys, and
// cannot escape its root; a nil fsys means the operating system's file system.
func ParseFile(fsys iofs.FS, filePath string) (*Bundle, error) {
	if fsys == nil {
		return parseFile(osSourceFS{}, filePath, "", nil)
	}
	return parseFile(virtualSourceFS{fsys: fsys}, filePath, "", fsys)
}

// ParseProject parses an Evie project directory (every top-level file declaring
// package main) and every package it imports by path, without running any
// code. It reads from fsys the way ParseFile does.
func ParseProject(fsys iofs.FS, dirPath string) (*Bundle, error) {
	if fsys == nil {
		return parseProject(osSourceFS{}, dirPath, nil)
	}
	return parseProject(virtualSourceFS{fsys: fsys}, dirPath, fsys)
}

// ParseSource parses a program given as source text, and every package it
// imports by path, without running any code. name is what errors call the
// source; path imports resolve against the working directory.
func ParseSource(name string, src []byte) (*Bundle, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return parseUnit(osSourceFS{}, diag.NewSource(name, src), name, dir, dir, nil)
}

// ---------------------------------------------------------------------------
// sourceFS abstraction (OS filesystem vs sandboxed io/fs.FS)
// ---------------------------------------------------------------------------

type sourceFS interface {
	abs(p string) (string, error)
	dir(p string) string
	canonical(p string) string
	display(p string) string
	stat(p string) (isDir bool, exists bool)
	readSource(p string) (*diag.Source, error)
	evFiles(dir string) ([]string, error)
	candidates(importPath, currentDir, root string) ([]string, error)
}

type osSourceFS struct{}

func (osSourceFS) abs(p string) (string, error) {
	return filepath.Abs(p)
}

func (osSourceFS) dir(p string) string {
	return filepath.Dir(p)
}

func (osSourceFS) canonical(p string) string {
	return canonicalKey(p)
}

func (osSourceFS) display(p string) string {
	return display(p)
}

func (osSourceFS) stat(p string) (bool, bool) {
	info, err := os.Stat(p)
	if err != nil {
		return false, false
	}
	return info.IsDir(), true
}

func (osSourceFS) readSource(p string) (*diag.Source, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", display(p), err)
	}
	return diag.NewSource(display(p), data), nil
}

func (osSourceFS) evFiles(dir string) ([]string, error) {
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

func (osSourceFS) candidates(importPath, currentDir, root string) ([]string, error) {
	var candidates []string

	if filepath.IsAbs(importPath) {
		candidates = append(candidates, importPath)
		if !strings.HasSuffix(importPath, ".ev") {
			candidates = append(candidates, importPath+".ev")
		}
		return candidates, nil
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
	if !isDotRelative(importPath) && root != "" && root != currentDir {
		add(root)
	}
	return candidates, nil
}

type virtualSourceFS struct {
	fsys iofs.FS
}

func cleanFSPath(p string) (string, error) {
	if p == "" {
		return ".", nil
	}
	slash := strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(slash, "/") || filepath.IsAbs(p) {
		return "", fmt.Errorf("path '%s' escapes filesystem root", p)
	}
	clean := path.Clean(slash)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path '%s' escapes filesystem root", p)
	}
	return clean, nil
}

func (v virtualSourceFS) abs(p string) (string, error) {
	return cleanFSPath(p)
}

func (v virtualSourceFS) dir(p string) string {
	return path.Dir(p)
}

func (v virtualSourceFS) canonical(p string) string {
	return p
}

func (v virtualSourceFS) display(p string) string {
	return p
}

func (v virtualSourceFS) stat(p string) (bool, bool) {
	if !iofs.ValidPath(p) {
		return false, false
	}
	info, err := iofs.Stat(v.fsys, p)
	if err != nil {
		return false, false
	}
	return info.IsDir(), true
}

func (v virtualSourceFS) readSource(p string) (*diag.Source, error) {
	data, err := iofs.ReadFile(v.fsys, p)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", p, err)
	}
	return diag.NewSource(p, data), nil
}

func (v virtualSourceFS) evFiles(dir string) ([]string, error) {
	entries, err := iofs.ReadDir(v.fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ev") {
			if dir == "." {
				files = append(files, entry.Name())
			} else {
				files = append(files, path.Join(dir, entry.Name()))
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

func (v virtualSourceFS) candidates(importPath, currentDir, root string) ([]string, error) {
	slash := strings.ReplaceAll(importPath, "\\", "/")
	if strings.HasPrefix(slash, "/") || filepath.IsAbs(importPath) {
		return nil, fmt.Errorf("import '%s' escapes filesystem root", importPath)
	}

	var candidates []string
	addFrom := func(base string) error {
		joined := path.Clean(path.Join(base, slash))
		if joined == ".." || strings.HasPrefix(joined, "../") {
			return fmt.Errorf("import '%s' escapes filesystem root", importPath)
		}
		if strings.HasSuffix(joined, ".ev") {
			candidates = append(candidates, joined)
		} else {
			candidates = append(candidates, joined+".ev", joined)
		}
		return nil
	}

	if currentDir == "" {
		currentDir = "."
	}
	if err := addFrom(currentDir); err != nil {
		return nil, err
	}
	if !isDotRelative(importPath) && root != "" && root != currentDir {
		if err := addFrom(root); err != nil {
			return nil, err
		}
	}
	return candidates, nil
}

// ---------------------------------------------------------------------------
// Graph-first parser & import resolver (zero VM execution / side effects)
// ---------------------------------------------------------------------------

type bundleError struct {
	diag.Report
}

func (e *bundleError) Error() string {
	return e.Report.Error()
}

func attachImportNote(err error, site importSite) error {
	if err == nil {
		return nil
	}
	var reporter interface{ Diagnostic() *diag.Report }
	if errors.As(err, &reporter) {
		r := reporter.Diagnostic()
		if r.Source != nil {
			note := diag.Note{
				Source: site.src,
				Pos:    site.pos,
				Text:   fmt.Sprintf("'%s' is imported here", site.importPath),
			}
			// Avoid duplicating the exact same note if an error bubbles through.
			if len(r.Notes) == 0 || r.Notes[len(r.Notes)-1] != note {
				r.Notes = append(r.Notes, note)
			}
			return err
		}
	}
	return &bundleError{Report: diag.Report{
		Message: fmt.Sprintf("cannot import '%s': %v", site.importPath, err),
		Source:  site.src,
		Pos:     site.pos,
	}}
}

type graphLoader struct {
	sfs        sourceFS
	root       string
	byKey      map[string]*parsedPackage
	loading    map[string]bool
	cycleTrace []string
	order      []*parsedPackage
}

func newGraphLoader(sfs sourceFS, root string) *graphLoader {
	return &graphLoader{
		sfs:     sfs,
		root:    root,
		byKey:   map[string]*parsedPackage{},
		loading: map[string]bool{},
	}
}

func parseFile(sfs sourceFS, filePath, root string, fsys iofs.FS) (*Bundle, error) {
	absPath, err := sfs.abs(filePath)
	if err != nil {
		return nil, err
	}
	dir := sfs.dir(absPath)
	if root == "" {
		root = dir
	} else if r, err := sfs.abs(root); err == nil {
		root = r
	}

	src, err := sfs.readSource(absPath)
	if err != nil {
		return nil, err
	}
	return parseUnit(sfs, src, absPath, dir, root, fsys)
}

// parseUnit parses the entry source of a program found at path, and its imports
// relative to dir
func parseUnit(sfs sourceFS, src *diag.Source, path, dir, root string, fsys iofs.FS) (*Bundle, error) {
	node, err := parser.ParseSource(src)
	if err != nil {
		return nil, err
	}
	if _, isPkg := node.(ast.Package); !isPkg {
		return &Bundle{
			script: &parsedUnit{path: path, src: src, node: node},
			root:   root,
			fsys:   fsys,
		}, nil
	}

	loader := newGraphLoader(sfs, root)
	key := sfs.canonical(path)
	if _, err := loader.loadUnits(key, path, dir, []parsedUnit{{path: path, src: src, node: node}}, true, nil); err != nil {
		return nil, err
	}
	return &Bundle{
		packages: loader.order,
		root:     root,
		fsys:     fsys,
	}, nil
}

func parseProject(sfs sourceFS, dirPath string, fsys iofs.FS) (*Bundle, error) {
	absDir, err := sfs.abs(dirPath)
	if err != nil {
		return nil, err
	}
	files, err := sfs.evFiles(absDir)
	if err != nil {
		return nil, err
	}

	var units []parsedUnit
	for _, file := range files {
		src, err := sfs.readSource(file)
		if err != nil {
			return nil, err
		}
		if parser.PeekPackageName(src) != "main" {
			continue
		}
		node, err := parser.ParseSource(src)
		if err != nil {
			return nil, err
		}
		units = append(units, parsedUnit{path: file, src: src, node: node})
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("no file in %s declares package main", sfs.display(absDir))
	}

	loader := newGraphLoader(sfs, absDir)
	dirKey := sfs.canonical(absDir)
	fileKeys := make([]string, len(units))
	for i, u := range units {
		fileKeys[i] = sfs.canonical(u.path)
		loader.loading[fileKeys[i]] = true
	}
	defer func() {
		for _, fk := range fileKeys {
			delete(loader.loading, fk)
		}
	}()

	if _, err := loader.loadUnits(dirKey, absDir, absDir, units, true, nil); err != nil {
		return nil, err
	}
	return &Bundle{
		packages: loader.order,
		root:     absDir,
		fsys:     fsys,
	}, nil
}

func (l *graphLoader) loadUnits(key, label, dir string, units []parsedUnit, allowMain bool, impSite *importSite) (*parsedPackage, error) {
	if existing, ok := l.byKey[key]; ok {
		return existing, nil
	}
	if l.loading[key] {
		cycle := append(slices.Clone(l.cycleTrace), label)
		return nil, fmt.Errorf("cyclic import detected: %s", strings.Join(cycle, " -> "))
	}

	// Validate package declarations across all files of this package.
	firstPkg, ok := units[0].node.(ast.Package)
	if !ok {
		return nil, fmt.Errorf("%s does not declare a package", units[0].src.Name)
	}
	pkgName := firstPkg.Name
	if !allowMain && pkgName == "main" {
		return nil, fmt.Errorf("%s declares package main, which cannot be imported", units[0].src.Name)
	}
	for _, u := range units[1:] {
		p, ok := u.node.(ast.Package)
		if !ok {
			return nil, fmt.Errorf("%s does not declare a package", u.src.Name)
		}
		if !allowMain && p.Name == "main" {
			return nil, fmt.Errorf("%s declares package main, which cannot be imported", u.src.Name)
		}
		if p.Name != pkgName {
			return nil, &bundleError{Report: diag.Report{
				Message: fmt.Sprintf("this file declares package '%s', but %s declares '%s'", p.Name, units[0].src.Name, pkgName),
				Source:  u.src,
				Pos:     p.Where(),
				Hint:    "every file of a package declares the same name",
			}}
		}
	}

	l.loading[key] = true
	l.cycleTrace = append(l.cycleTrace, label)
	defer func() {
		delete(l.loading, key)
		l.cycleTrace = l.cycleTrace[:len(l.cycleTrace)-1]
	}()

	pkg := &parsedPackage{
		key:        key,
		name:       pkgName,
		origin:     units[0].src.Name,
		dir:        dir,
		units:      units,
		deps:       map[string]string{},
		importedBy: impSite,
	}

	// Resolve all imports declared across this package's files in source order.
	seenImports := map[string]bool{}
	boundNames := map[string]string{} // local bound name -> import path (for path & aliased imports)
	for _, u := range units {
		p := u.node.(ast.Package)
		for _, spec := range p.Imports {
			dedupKey := spec.Path + "\x00" + spec.Alias
			if seenImports[dedupKey] {
				continue
			}
			seenImports[dedupKey] = true
			site := importSite{src: u.src, pos: spec.Pos, importPath: spec.Path}

			if isPathImport(spec.Path) {
				dep, err := l.resolvePath(spec.Path, dir, site)
				if err != nil {
					return nil, attachImportNote(err, site)
				}
				bindName := spec.Alias
				if bindName == "" {
					bindName = dep.name
				}
				if prevPath, clash := boundNames[bindName]; clash && prevPath != spec.Path {
					return nil, &bundleError{Report: diag.Report{
						Message: fmt.Sprintf("import '%s' binds as '%s', which is already bound by '%s'", spec.Path, bindName, prevPath),
						Source:  u.src,
						Pos:     spec.Pos,
						Hint:    fmt.Sprintf("use 'as <alias>' to give one a different name, e.g. imports(\"%s\" as %s2)", spec.Path, bindName),
					}}
				}
				boundNames[bindName] = spec.Path
				pkg.deps[spec.Path] = dep.key
			} else {
				if spec.Alias != "" {
					if prevPath, clash := boundNames[spec.Alias]; clash && prevPath != spec.Path {
						return nil, &bundleError{Report: diag.Report{
							Message: fmt.Sprintf("import '%s' binds as '%s', which is already bound by '%s'", spec.Path, spec.Alias, prevPath),
							Source:  u.src,
							Pos:     spec.Pos,
							Hint:    fmt.Sprintf("use 'as <alias>' to give one a different name, e.g. imports(\"%s\" as %s2)", spec.Path, spec.Alias),
						}}
					}
					boundNames[spec.Alias] = spec.Path
				}
				// Record if a local file/dir of the same name exists next to this package
				// so shadowed-import hints work even when the Bundle runs elsewhere.
				if cands, err := l.sfs.candidates(spec.Path, dir, ""); err == nil {
					for _, cand := range cands {
						if _, exists := l.sfs.stat(cand); exists {
							if pkg.shadowed == nil {
								pkg.shadowed = map[string]string{}
							}
							pkg.shadowed[spec.Path] = l.sfs.display(cand)
							break
						}
					}
				}
			}
		}
	}

	l.byKey[key] = pkg
	l.order = append(l.order, pkg)
	return pkg, nil
}

func (l *graphLoader) resolvePath(importPath, currentDir string, site importSite) (*parsedPackage, error) {
	candidates, err := l.sfs.candidates(importPath, currentDir, l.root)
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		isDir, exists := l.sfs.stat(candidate)
		if !exists {
			continue
		}
		if isDir {
			return l.loadDir(candidate, site)
		}
		return l.loadFile(candidate, site)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("package '%s' not found", importPath)
	}
	tried := make([]string, len(candidates))
	for i, c := range candidates {
		tried[i] = l.sfs.display(c)
	}
	return nil, fmt.Errorf("not found; looked for %s", strings.Join(tried, " and "))
}

func (l *graphLoader) loadFile(filePath string, site importSite) (*parsedPackage, error) {
	absPath, err := l.sfs.abs(filePath)
	if err != nil {
		return nil, err
	}
	key := l.sfs.canonical(absPath)
	if existing, ok := l.byKey[key]; ok {
		return existing, nil
	}
	if l.loading[key] {
		cycle := append(slices.Clone(l.cycleTrace), absPath)
		return nil, fmt.Errorf("cyclic import detected: %s", strings.Join(cycle, " -> "))
	}

	src, err := l.sfs.readSource(absPath)
	if err != nil {
		return nil, err
	}
	if parser.PeekPackageName(src) == "main" {
		return nil, fmt.Errorf("%s declares package main, which cannot be imported", src.Name)
	}
	node, err := parser.ParseSource(src)
	if err != nil {
		return nil, err
	}
	siteCopy := site
	return l.loadUnits(key, absPath, l.sfs.dir(absPath), []parsedUnit{{path: absPath, src: src, node: node}}, false, &siteCopy)
}

func (l *graphLoader) loadDir(dirPath string, site importSite) (*parsedPackage, error) {
	absDir, err := l.sfs.abs(dirPath)
	if err != nil {
		return nil, err
	}
	dirKey := l.sfs.canonical(absDir)
	if existing, ok := l.byKey[dirKey]; ok {
		return existing, nil
	}
	if l.loading[dirKey] {
		cycle := append(slices.Clone(l.cycleTrace), absDir)
		return nil, fmt.Errorf("cyclic import detected: %s", strings.Join(cycle, " -> "))
	}

	files, err := l.sfs.evFiles(absDir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .ev files in %s", l.sfs.display(absDir))
	}

	fileKeys := make([]string, len(files))
	units := make([]parsedUnit, len(files))
	for i, file := range files {
		fileKeys[i] = l.sfs.canonical(file)
		if l.loading[fileKeys[i]] {
			cycle := append(slices.Clone(l.cycleTrace), file)
			return nil, fmt.Errorf("cyclic import detected: %s", strings.Join(cycle, " -> "))
		}
		src, err := l.sfs.readSource(file)
		if err != nil {
			return nil, err
		}
		if parser.PeekPackageName(src) == "main" {
			return nil, fmt.Errorf("%s declares package main, which cannot be imported", src.Name)
		}
		node, err := parser.ParseSource(src)
		if err != nil {
			return nil, err
		}
		units[i] = parsedUnit{path: file, src: src, node: node}
	}

	for _, fk := range fileKeys {
		l.loading[fk] = true
	}
	defer func() {
		for _, fk := range fileKeys {
			delete(l.loading, fk)
		}
	}()

	siteCopy := site
	return l.loadUnits(dirKey, absDir, absDir, units, false, &siteCopy)
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

func isDotRelative(p string) bool {
	return p == "." || p == ".." ||
		strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") ||
		strings.HasPrefix(p, `.\`) || strings.HasPrefix(p, `..\`)
}
