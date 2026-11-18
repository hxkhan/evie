package evie_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

func TestFSRunFileAndProject(t *testing.T) {
	fsys := fstest.MapFS{
		"constants.ev": &fstest.MapFile{Data: []byte(`package constants
const BASE := 100
`)},
		"calc/add.ev": &fstest.MapFile{Data: []byte(`package calc
fn add(a, b) => a + b
`)},
		"calc/mul.ev": &fstest.MapFile{Data: []byte(`package calc
fn mul(a, b) => a * b
`)},
		"sub/worker.ev": &fstest.MapFile{Data: []byte(`package worker imports("../constants", "../calc")
fn compute(x) => calc.add(constants.BASE, calc.mul(x, 2))
`)},
		"app.ev": &fstest.MapFile{Data: []byte(`package main imports("./sub/worker")
fn main() => greet(worker.compute(21))
`)},
		"greet.ev": &fstest.MapFile{Data: []byte(`package main
fn greet(n) => "result=" + string(n)
`)},
	}

	// RunProject via Options.FS
	val, err := evie.New(evie.Options{FS: fsys}).RunProject(".")
	expectResult(t, val, err, "result=142")

	// RunFile via Options.FS on a single-file entry point
	singleFS := fstest.MapFS{
		"util.ev": &fstest.MapFile{Data: []byte(`package util
fn twice(x) => x * 2
`)},
		"entry.ev": &fstest.MapFile{Data: []byte(`package main imports("./util")
fn main() => util.twice(21)
`)},
	}
	val, err = evie.New(evie.Options{FS: singleFS}).RunFile("entry.ev")
	expectResult(t, val, err, "42")

	// Inline Eval with Options.FS resolves relative imports inside FS
	eng := evie.New(evie.Options{FS: singleFS})
	if _, err := eng.Eval(`package main imports("./util")
	fn main() => util.twice(10)`); err != nil {
		t.Fatalf("Eval with FS failed: %v", err)
	}
	val, err = eng.Call("main.main")
	expectResult(t, val, err, "20")
}

func TestFSImportSandboxBlocksEscape(t *testing.T) {
	cases := map[string]string{
		"parent":   `package main imports("../secret")`,
		"deep":     `package main imports("./sub/../../secret")`,
		"absolute": `package main imports("/etc/passwd")`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"main.ev": &fstest.MapFile{Data: []byte(src + "\nfn main() => 0\n")},
			}
			_, err := evie.ParseFileFS(fsys, "main.ev")
			expectError(t, err, "escapes filesystem root")

			_, err = evie.New(evie.Options{FS: fsys}).RunFile("main.ev")
			expectError(t, err, "escapes filesystem root")
		})
	}

	// Entry path itself cannot escape FS root either
	fsys := fstest.MapFS{"main.ev": &fstest.MapFile{Data: []byte("package main\nfn main() => 1\n")}}
	if _, err := evie.ParseFileFS(fsys, "../main.ev"); err == nil || !strings.Contains(err.Error(), "escapes filesystem root") {
		t.Fatalf("expected escape error for ../main.ev, got %v", err)
	}
}

func TestBundleParseOnceRunConcurrently(t *testing.T) {
	fsys := fstest.MapFS{
		"state.ev": &fstest.MapFile{Data: []byte(`package state
var count := 0
fn bump(n) {
	count += n
	return count
}
`)},
		"main.ev": &fstest.MapFile{Data: []byte(`package main imports("io", "./state")
var initRan := state.bump(10)
fn main(tag) {
	total := state.bump(5)
	io.println(tag, total)
	return total
}
`)},
	}

	// Phase 1: parse once without executing anything
	bundle, err := evie.ParseProjectFS(fsys, ".")
	if err != nil {
		t.Fatalf("ParseProjectFS failed: %v", err)
	}

	if got := strings.Join(bundle.Packages(), ","); got != "state,main" {
		t.Fatalf("Packages() = %q, want %q", got, "state,main")
	}
	if got := strings.Join(bundle.Files(), ","); got != "state.ev,main.ev" {
		t.Fatalf("Files() = %q, want %q", got, "state.ev,main.ev")
	}

	// Phase 2: instantiate concurrently in 16 isolated engines — each must see its
	// own independent mutable state (count starts at 0 -> +10 in init -> +5 in main = 15)
	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			var stdout bytes.Buffer
			tag := fmt.Sprintf("w%d", id)
			val, err := bundle.Run(evie.Options{
				Args:   []string{tag},
				Stdout: &stdout,
			})
			if err != nil {
				errs <- fmt.Errorf("worker %d: %w", id, err)
				return
			}
			if val.String() != "15" {
				errs <- fmt.Errorf("worker %d: got %s, want 15", id, val)
				return
			}
			if want := tag + " 15\n"; stdout.String() != want {
				errs <- fmt.Errorf("worker %d: stdout %q, want %q", id, stdout.String(), want)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestBundleLoadCallDumpAndContext(t *testing.T) {
	fsys := fstest.MapFS{
		"lib.ev": &fstest.MapFile{Data: []byte(`package lib
fn double(x) => x * 2
`)},
		"main.ev": &fstest.MapFile{Data: []byte(`package main imports("./lib", "host")
fn compute(x) => lib.double(x) + host.offset
fn main() {
	throw error("main should not run on LoadBundle")
}
`)},
	}

	bundle, err := evie.ParseFileFS(fsys, "main.ev")
	if err != nil {
		t.Fatalf("ParseFileFS failed: %v", err)
	}

	host := vm.NewPackage("host")
	host.Set("offset", vm.BoxNumber(2))

	eng := evie.New(evie.Options{
		Packages: []*vm.Package{host},
		Debug:    evie.Debug{DebugInfo: true},
	})
	if err := eng.LoadBundle(bundle); err != nil {
		t.Fatalf("LoadBundle failed: %v", err)
	}
	val, err := eng.Call("main.compute", 20)
	expectResult(t, val, err, "42")

	var dump bytes.Buffer
	eng.Dump(&dump)
	if !strings.Contains(dump.String(), "package lib") || !strings.Contains(dump.String(), "package main") {
		t.Fatalf("unexpected Dump output:\n%s", dump.String())
	}

	// RunContext stops a runaway bundle cleanly
	stuckFS := fstest.MapFS{
		"main.ev": &fstest.MapFile{Data: []byte("package main\nfn main() { while true {} }\n")},
	}
	stuckBundle, err := evie.ParseFileFS(stuckFS, "main.ev")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = stuckBundle.RunContext(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestBundleShadowHintAndCycleDetection(t *testing.T) {
	shadowFS := fstest.MapFS{
		"math.ev": &fstest.MapFile{Data: []byte("package mymath\nfn mine() => 1\n")},
		"main.ev": &fstest.MapFile{Data: []byte("package main imports(\"math\")\nfn main() => math.mine()\n")},
	}
	bundle, err := evie.ParseFileFS(shadowFS, "main.ev")
	if err != nil {
		t.Fatalf("ParseFileFS failed: %v", err)
	}
	_, err = bundle.Run()
	if err == nil {
		t.Fatal("expected error calling math.mine()")
	}
	if rendered := evie.Format(err, false); !strings.Contains(rendered, `import "./math"`) {
		t.Fatalf("expected shadowed-import hint, got:\n%s", rendered)
	}

	// Cycles are rejected during ParseFileFS before any code runs
	cycleFS := fstest.MapFS{
		"a.ev":    &fstest.MapFile{Data: []byte("package a imports(\"./b\")\nvar boom := throw error(\"must not run\")\n")},
		"b.ev":    &fstest.MapFile{Data: []byte("package b imports(\"./a\")\nfn x() => 1\n")},
		"main.ev": &fstest.MapFile{Data: []byte("package main imports(\"./a\")\nfn main() => 0\n")},
	}
	_, err = evie.ParseFileFS(cycleFS, "main.ev")
	expectError(t, err, "cyclic import detected")
}

func TestCanonicalModuleIdentityAndImportAliases(t *testing.T) {
	// 1. Two packages declaring the same name coexist cleanly:
	//    - without aliases when imported by different packages
	//    - with `as` aliases when imported into the same package
	fsys := fstest.MapFS{
		"db/util.ev":   &fstest.MapFile{Data: []byte("package util\nvar n := 1\nfn tag() => \"db:\" + string(n)\n")},
		"http/util.ev": &fstest.MapFile{Data: []byte("package util\nvar n := 2\nfn tag() => \"http:\" + string(n)\n")},
		"a.ev":         &fstest.MapFile{Data: []byte("package a imports(\"./db/util\")\nfn run() {\n\tutil.n += 10\n\treturn util.tag()\n}\n")},
		"b.ev":         &fstest.MapFile{Data: []byte("package b imports(\"./http/util\")\nfn run() => util.tag()\n")},
		"main.ev": &fstest.MapFile{Data: []byte(`package main imports(
			"./a",
			"./b",
			"./db/util" as dbUtil,
			"./http/util" as httpUtil,
		)
		fn main() {
			// 'as' is contextual and remains a valid variable name
			as := 100
			return [a.run(), b.run(), dbUtil.n + httpUtil.n + as]
		}`)},
	}

	bundle, err := evie.ParseFileFS(fsys, "main.ev")
	if err != nil {
		t.Fatalf("ParseFileFS failed: %v", err)
	}
	eng := evie.New(evie.Options{Debug: evie.Debug{DebugInfo: true}})
	val, err := eng.RunBundle(bundle)
	// a.run() mutates shared db/util.n from 1 -> 11; http/util.n stays 2; 11 + 2 + 100 = 113
	expectResult(t, val, err, `["db:11", "http:2", 113]`)

	// Dump should list both distinct util modules and the aliased imports
	var dump bytes.Buffer
	eng.Dump(&dump)
	dumpStr := dump.String()
	for _, want := range []string{
		"package util (db/util.ev)",
		"package util (http/util.ev)",
		"./db/util as dbUtil",
		"./http/util as httpUtil",
	} {
		if !strings.Contains(dumpStr, want) {
			t.Errorf("Dump missing %q:\n%s", want, dumpStr)
		}
	}

	// 2. Stdlib "math" and local "./math" (both declaring `package math`) coexist in either order
	for _, order := range []string{
		`package main imports("math", "./math" as localMath)`,
		`package main imports("./math" as localMath, "math")`,
	} {
		mathFS := fstest.MapFS{
			"math.ev": &fstest.MapFile{Data: []byte("package math\nfn extra() => 40\n")},
			"main.ev": &fstest.MapFile{Data: []byte(order + "\nfn main() => math.sqrt(4) + localMath.extra()\n")},
		}
		val, err := evie.New(evie.Options{FS: mathFS}).RunFile("main.ev")
		expectResult(t, val, err, "42")
	}

	// 3. Importing both stdlib "math" and local "./math" without an alias fails with a clear hint
	clashFS := fstest.MapFS{
		"math.ev": &fstest.MapFile{Data: []byte("package math\nfn extra() => 40\n")},
		"main.ev": &fstest.MapFile{Data: []byte("package main imports(\"math\", \"./math\")\nfn main() => 0\n")},
	}
	_, err = evie.New(evie.Options{FS: clashFS}).RunFile("main.ev")
	expectError(t, err, "binds as 'math'", "already bound by 'math'")
	if rendered := evie.Format(err, false); !strings.Contains(rendered, "as <alias>") {
		t.Fatalf("expected alias hint in rendered error:\n%s", rendered)
	}

	// 4. Using the original declared name after aliasing gives a helpful hint
	hintFS := fstest.MapFS{
		"math.ev": &fstest.MapFile{Data: []byte("package mymath\nfn extra() => 1\n")},
		"main.ev": &fstest.MapFile{Data: []byte("package main imports(\"./math\" as m)\nfn main() => mymath.extra()\n")},
	}
	_, err = evie.New(evie.Options{FS: hintFS}).RunFile("main.ev")
	expectError(t, err, "cannot find 'mymath'")
	if rendered := evie.Format(err, false); !strings.Contains(rendered, "bound as 'm'") {
		t.Fatalf("expected bound-as hint in rendered error:\n%s", rendered)
	}

	// 5. Invalid alias (keyword) is a syntax error
	badAliasFS := fstest.MapFS{
		"main.ev": &fstest.MapFile{Data: []byte("package main imports(\"math\" as fn)\nfn main() => 0\n")},
	}
	_, err = evie.ParseFileFS(badAliasFS, "main.ev")
	expectError(t, err, "keyword")
}
