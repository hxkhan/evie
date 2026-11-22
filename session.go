package evie

import (
	"cmp"
	"context"
	"fmt"
	"sort"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
	"github.com/hxkhan/evie/vm"
)

// Session runs Evie code incrementally against a persistent package inside an
// Engine. Variables, functions, classes, and imports persist across Eval calls,
// trailing expressions return their value (and bind `_`), and standard library
// packages referenced as `pkg.sym` auto-import when allowed by Options.
//
// A Session can be opened on a fresh Engine (for a REPL, notebook, or stateful
// scripting) or on an Engine that already loaded a program (for a live debug
// console attached to package main).
type Session struct {
	eng  *Engine
	vm   *vm.Session
	turn int
}

// NewSession opens an incremental evaluation session on e's "main" package.
// If e has already loaded a compiled "main" package, the session attaches to
// it so existing globals and functions are directly accessible.
func (e *Engine) NewSession() *Session {
	return &Session{
		eng: e,
		vm:  e.vm.NewSession("main"),
	}
}

// Eval parses and executes one snippet against the session. When hasVal is
// true, val is the result of the trailing expression (or an explicit return).
func (s *Session) Eval(code string) (val vm.Value, hasVal bool, err error) {
	return s.EvalContext(context.Background(), code)
}

// EvalContext is Eval bound to ctx.
func (s *Session) EvalContext(ctx context.Context, code string) (val vm.Value, hasVal bool, err error) {
	if err := s.eng.usable(); err != nil {
		return vm.Value{}, false, err
	}
	s.turn++
	src := diag.NewSource(fmt.Sprintf("<session:%d>", s.turn), []byte(code))
	node, err := parser.ParseSource(src)
	if err != nil {
		return vm.Value{}, false, err
	}

	s.autoImportStd(node)
	return s.vm.EvalContext(ctx, src, node)
}

// Compile parses and compiles code against the session without running its
// top-level statements (intended for validation or Engine.Dump).
func (s *Session) Compile(code string) error {
	s.turn++
	src := diag.NewSource(fmt.Sprintf("<session:%d>", s.turn), []byte(code))
	node, err := parser.ParseSource(src)
	if err != nil {
		return err
	}

	s.autoImportStd(node)
	return s.vm.Compile(src, node)
}

// Import resolves importPath (a stdlib name, host package, or relative file /
// directory path) and binds it in the session under alias (or the package's
// declared name when alias is empty).
func (s *Session) Import(importPath, alias string) (*vm.Package, error) {
	prevRoot := s.eng.root
	if s.eng.opts.FS == nil && s.eng.root == "" {
		s.eng.root = "."
		defer func() { s.eng.root = prevRoot }()
	}
	dep, err := s.eng.resolve(importPath)
	if err != nil {
		return nil, err
	}
	bindName := alias
	if bindName == "" {
		bindName = dep.Name()
	}
	s.vm.BindImport(bindName, importPath, dep)
	return dep, nil
}

// LoadFile reads a .ev file, inside Options.FS when one is set, and evaluates
// its statements into the session; its imports resolve relative to the file.
// A file that declares a package other than main is imported as that package.
func (s *Session) LoadFile(filePath string) (vm.Value, bool, error) {
	if err := s.eng.usable(); err != nil {
		return vm.Value{}, false, err
	}
	b, err := parseFile(s.eng.sfs(), filePath, "", s.eng.opts.FS)
	if err != nil {
		return vm.Value{}, false, err
	}
	if b.script != nil {
		s.autoImportStd(b.script.node)
		return s.vm.EvalContext(context.Background(), b.script.src, b.script.node)
	}

	// the entry package comes last; the packages it imports by path come before it
	entry := b.packages[len(b.packages)-1]
	for _, p := range b.packages[:len(b.packages)-1] {
		if _, loaded := s.eng.bundleLoaded[p.key]; loaded {
			continue
		}
		if err := s.eng.compileParsedPackage(context.Background(), p); err != nil {
			return vm.Value{}, false, err
		}
	}
	if entry.name != "main" {
		if _, loaded := s.eng.bundleLoaded[entry.key]; !loaded {
			if err := s.eng.compileParsedPackage(context.Background(), entry); err != nil {
				return vm.Value{}, false, err
			}
		}
		pkg := s.eng.bundleLoaded[entry.key]
		s.vm.BindImport(pkg.Name(), filePath, pkg)
		return pkg.Box(), false, nil
	}

	// a main file runs in the session: bind its imports, then run its statements
	unit := entry.units[0]
	pkg := unit.node.(ast.Package)
	for _, spec := range pkg.Imports {
		if key, ok := entry.deps[spec.Path]; ok {
			dep := s.eng.bundleLoaded[key]
			s.vm.BindImport(cmp.Or(spec.Alias, dep.Name()), spec.Path, dep)
			continue
		}
		if _, err := s.Import(spec.Path, spec.Alias); err != nil {
			return vm.Value{}, false, err
		}
	}
	node := ast.Block{Pos: pkg.Pos, Code: pkg.Code}
	s.autoImportStd(node)
	return s.vm.EvalContext(context.Background(), unit.src, node)
}

// Symbols returns every binding currently defined in the session, sorted by name.
func (s *Session) Symbols() []vm.Symbol {
	return s.vm.Symbols()
}

// autoImportStd binds any standard library package referenced as `pkg.member`
// in root when allowed by Options and not shadowed by a session or local
// declaration.
func (s *Session) autoImportStd(root ast.Node) {
	if s.eng.opts.NoStdlib {
		return
	}

	declared := map[string]bool{}
	var collectDecls func(n ast.Node)
	collectDecls = func(n ast.Node) {
		switch n := n.(type) {
		case ast.Block:
			for _, stmt := range n.Code {
				collectDecls(stmt)
			}
		case ast.Decl:
			declared[n.Name] = true
		case ast.Fn:
			if n.Name != "" {
				declared[n.Name] = true
			}
		case ast.ClassDefinition:
			if n.Name != "" {
				declared[n.Name] = true
			}
		case ast.For:
			if n.IndexVar != "" {
				declared[n.IndexVar] = true
			}
			if n.ItemVar != "" {
				declared[n.ItemVar] = true
			}
		}
	}
	collectDecls(root)

	needed := map[string]bool{}
	ast.Walk(root, func(n ast.Node) bool {
		if fa, ok := n.(ast.FieldAccess); ok {
			if id, ok := fa.Lhs.(ast.Ident); ok {
				if _, isStd := stdlib[id.Name]; isStd && !declared[id.Name] {
					if s.eng.stdAllow == nil || s.eng.stdAllow[id.Name] {
						if !s.vm.Has(id.Name) {
							needed[id.Name] = true
						}
					}
				}
				return false
			}
		}
		return true
	})

	if len(needed) == 0 {
		return
	}
	names := make([]string, 0, len(needed))
	for name := range needed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		_, _ = s.Import(name, "")
	}
}
