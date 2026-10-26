package evie_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/diag"
)

// writeTree writes files given by slash-separated paths under a temporary
// directory and returns the directory
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func expectResult(t *testing.T, got interface{ String() string }, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != want {
		t.Fatalf("got %s, want %s", got.String(), want)
	}
}

func expectError(t *testing.T, err error, want ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error mentioning %q", want)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Fatalf("error %q does not mention %q", err.Error(), w)
		}
	}
}

// the files of a package see each other's declarations whatever their order
func TestLoaderPackageAcrossFiles(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"calc/a_ops.ev":     "package calc\nfn double(x) => twice(x)\nfn bump() {\n\tcount += 1\n\treturn count\n}",
		"calc/b_helpers.ev": "package calc\nfn twice(x) => x * 2\nvar count := 10",
		"main.ev":           "package main imports(\"./calc\")\nfn main() {\n\tcalc.bump()\n\treturn [calc.double(21), calc.bump()]\n}",
	})
	val, err := evie.RunFile(filepath.Join(dir, "main.ev"))
	expectResult(t, val, err, "[42, 12]")
}

// a project's main package can span the files at its top level, next to
// packages it imports by path, and needs no file called main.ev
func TestLoaderProjectMainAcrossFiles(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"app.ev":    "package main imports(\"./helper\")\nfn main() => greet(helper.name())",
		"greet.ev":  "package main\nfn greet(who) => \"hello \" + who",
		"helper.ev": "package helper\nfn name() => \"project\"",
	})
	val, err := evie.RunProject(dir)
	expectResult(t, val, err, "hello project")

	empty := writeTree(t, map[string]string{"lib.ev": "package lib"})
	_, err = evie.RunProject(empty)
	expectError(t, err, "declares package main")
}

// an import path means the file next to the importer, not a file another
// package imported under the same string
func TestLoaderSamePathFromDifferentFolders(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a/util.ev": "package autil\nfn name() => \"a/util\"",
		"a/a.ev":    "package a imports(\"./util\")\nfn who() => autil.name()",
		"b/util.ev": "package butil\nfn name() => \"b/util\"",
		"b/b.ev":    "package b imports(\"./util\")\nfn who() => butil.name()",
		"main.ev":   "package main imports(\"./a/a\", \"./b/b\")\nfn main() => [a.who(), b.who()]",
	})
	val, err := evie.RunFile(filepath.Join(dir, "main.ev"))
	expectResult(t, val, err, `["a/util", "b/util"]`)
}

// packages are told apart by where they come from; a second package with a
// name that is taken, a folder of mixed packages, or an imported main is an error
func TestLoaderPackageNames(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"lib1/util.ev": "package util\nfn one() => 1",
		"lib2/util.ev": "package util\nfn two() => 2",
		"dup.ev":       "package main imports(\"./lib1/util\", \"./lib2/util\")\nfn main() => 0",
		"mixed/a.ev":   "package alpha",
		"mixed/b.ev":   "package beta",
		"mix.ev":       "package main imports(\"./mixed\")\nfn main() => 0",
		"other.ev":     "package main\nfn helper() => 1",
		"imp.ev":       "package main imports(\"./other\")\nfn main() => 0",
	})
	_, err := evie.RunFile(filepath.Join(dir, "dup.ev"))
	expectError(t, err, "package 'util' is already loaded from", "lib1")
	_, err = evie.RunFile(filepath.Join(dir, "mix.ev"))
	expectError(t, err, "declares package 'beta'", "declares 'alpha'")
	_, err = evie.RunFile(filepath.Join(dir, "imp.ev"))
	expectError(t, err, "declares package main, which cannot be imported")
}

// an error inside an imported package is reported in that package's file,
// with a note at the import
func TestLoaderErrorsInsideImports(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"compile/broken.ev": "package broken\nfn f() => undefinedName + 1",
		"compile/main.ev":   "package main imports(\"./broken\")\nfn main() => broken.f()",
		"runtime/broken.ev": "package broken\nconfig := load()\nfn load() {\n\treturn 1 + \"x\"\n}",
		"runtime/main.ev":   "package main imports(\"./broken\")\nfn main() => broken.config",
		"syntax/broken.ev":  "package broken\nfn f() => (1 +",
		"syntax/main.ev":    "package main imports(\"./broken\")\nfn main() => 0",
	})
	for _, kind := range []string{"compile", "runtime", "syntax"} {
		_, err := evie.RunFile(filepath.Join(dir, kind, "main.ev"))
		var reporter interface{ Diagnostic() *diag.Report }
		if !errors.As(err, &reporter) {
			t.Fatalf("%s: expected a diagnostic, got %v", kind, err)
		}
		r := reporter.Diagnostic()
		if !strings.HasSuffix(r.Source.Name, "broken.ev") {
			t.Fatalf("%s: reported in %s, not broken.ev", kind, r.Source.Name)
		}
		if len(r.Notes) == 0 || !strings.Contains(r.Notes[len(r.Notes)-1].Text, "'./broken' is imported here") {
			t.Fatalf("%s: no note at the import: %+v", kind, r.Notes)
		}
	}
}

// mistakes in how a package is imported get a hint
func TestLoaderImportHints(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"helpers/strutil.ev": "package text\nfn shout(s) => s.toUpper()",
		"alias.ev":           "package main imports(\"./helpers/strutil\")\nfn main() => strutil.shout(\"hi\")",
		"math.ev":            "package mymath\nfn mine() => 1",
		"shadow.ev":          "package main imports(\"math\")\nfn main() => math.mine()",
		"missing.ev":         "package main imports(\"./nope\")\nfn main() => 0",
	})
	_, err := evie.RunFile(filepath.Join(dir, "alias.ev"))
	expectError(t, err, "cannot find 'strutil'")
	var reporter interface{ Diagnostic() *diag.Report }
	if errors.As(err, &reporter); !strings.Contains(reporter.Diagnostic().Hint, "bound as 'text'") {
		t.Fatalf("no hint about the bound name: %q", reporter.Diagnostic().Hint)
	}

	_, err = evie.RunFile(filepath.Join(dir, "shadow.ev"))
	if errors.As(err, &reporter); !strings.Contains(reporter.Diagnostic().Hint, `import "./math"`) {
		t.Fatalf("no hint about the local math.ev: %v", err)
	}

	_, err = evie.RunFile(filepath.Join(dir, "missing.ev"))
	expectError(t, err, "cannot import './nope': not found; looked for", "nope.ev")
}

// an engine runs one program and loads each package once
func TestEngineRunsOneProgram(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"one.ev": "package main\nfn main() => \"first\"",
		"two.ev": "package main\nfn other() => 2",
	})
	eng := evie.New()
	val, err := eng.RunFile(filepath.Join(dir, "one.ev"))
	expectResult(t, val, err, "first")
	_, err = eng.RunFile(filepath.Join(dir, "two.ev"))
	expectError(t, err, "already ran a program")
	_, err = eng.RunProject(dir)
	expectError(t, err, "already ran a program")

	src := []byte("package plugin\nfn version() => 1")
	if _, err := eng.Load("plugin.ev", src); err != nil {
		t.Fatal(err)
	}
	_, err = eng.Load("plugin.ev", src)
	expectError(t, err, "package 'plugin' is already loaded from plugin.ev")
}
