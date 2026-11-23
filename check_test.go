package evie_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

func checkFiles(t *testing.T, files map[string]string, out *bytes.Buffer) error {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, src := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(src)}
	}
	b, err := evie.ParseFile(fsys, "main.ev")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	eng := evie.New(evie.Options{FS: fsys, Stdout: out})
	defer eng.Close()
	return eng.Check(b)
}

// checking compiles everything and runs nothing: not the top-level code of
// the program or of what it imports, not main
func TestCheckRunsNothing(t *testing.T) {
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- checkFiles(t, map[string]string{
			"main.ev": `package main imports("io", "./util")
io.println("main's top level ran")
var i := 0
while true { i += 1 }
fn main() { io.println("main ran", util.twice(2)) }`,
			"util.ev": `package util imports("io")
io.println("util's top level ran")
const base := 10
fn twice(n) => n * 2 + base - base`,
		}, &out)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a correct program should check cleanly: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("checking ran the program's endless top-level loop")
	}
	if out.Len() != 0 {
		t.Fatalf("checking ran code that printed %q", out.String())
	}
}

// the errors are the ones a run would stop with before starting
func TestCheckReportsCompileErrors(t *testing.T) {
	cases := []struct {
		files map[string]string
		want  string
	}{
		{map[string]string{"main.ev": "package main\nfn main() { return nope }"}, "cannot find 'nope'"},
		{map[string]string{"main.ev": "package main\nconst x := 1\nfn main() { x = 2 }"}, "x"},
		// an error in an imported file
		{map[string]string{
			"main.ev": "package main imports(\"./util\")\nfn main() => util.f()",
			"util.ev": "package util\nfn f() => undefinedThing",
		}, "cannot find 'undefinedThing'"},
		// a bare script is compiled, not run
		{map[string]string{"main.ev": "x := 1\nreturn x + y"}, "cannot find 'y'"},
	}
	for _, c := range cases {
		var out bytes.Buffer
		err := checkFiles(t, c.files, &out)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: got %v, want an error mentioning %q", c.files, err, c.want)
		}
		if out.Len() != 0 {
			t.Errorf("%v: checking printed %q", c.files, out.String())
		}
	}
}

// a member an imported package lacks is a compile error however it is used,
// even in a function that never runs, and with or without the fast paths
func TestMissingPackageMemberIsACompileError(t *testing.T) {
	cases := []struct {
		files map[string]string
		want  string // "" when the program is fine
	}{
		{map[string]string{"main.ev": "package main imports(\"math\")\nfn f() => math.pie\nfn main() => 1"}, "no member 'pie'"},
		{map[string]string{"main.ev": "package main imports(\"math\")\nfn f() => math.sqr(2)\nfn main() => 1"}, "no member 'sqr'"},
		{map[string]string{"main.ev": "package main imports(\"math\")\nfn f() { math.pie = 3 }\nfn main() => 1"}, "no member 'pie'"},
		{map[string]string{"main.ev": "package main imports(\"math\")\nfn f() { math.pie += 1 }\nfn main() => 1"}, "no member 'pie'"},
		{map[string]string{
			"main.ev": "package main imports(\"./util\")\nfn f() => util.missing()\nfn main() => 1",
			"util.ev": "package util\nfn present() => 1",
		}, "no member 'missing'"},
		// a local of the same name is not the package
		{map[string]string{"main.ev": "package main imports(\"math\")\nfn f() { math := {sqr: 1}\n return math.sqr }\nfn main() => f()"}, ""},
		{map[string]string{"main.ev": "package main imports(\"math\")\nfn f(math) => math.anything()\nfn main() => 1"}, ""},
		{map[string]string{"main.ev": "package main imports(\"math\")\nfn main() => math.sqrt(4)"}, ""},
	}
	for _, c := range cases {
		var out bytes.Buffer
		err := checkFiles(t, c.files, &out)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%v: should compile, got %v", c.files, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%v: got %v, want an error mentioning %q", c.files, err, c.want)
		}
	}

	// the error suggests the member that was meant
	_, err := evie.New().Eval("package main imports(\"math\")\nfn f() => math.sqr(2)")
	if err == nil || !strings.Contains(evie.Format(err, false), "'sqrt'") {
		t.Errorf("want a hint naming sqrt, got %v", evie.Format(err, false))
	}
}

// compileErrors lists the messages of the compile errors err holds
func compileErrors(err error) []string {
	var list vm.CompileErrors
	if errors.As(err, &list) {
		var msgs []string
		for _, e := range list {
			msgs = append(msgs, fmt.Sprintf("%s:%d %s", e.Source.Name, e.Pos.Line, e.Message))
		}
		return msgs
	}
	var one *vm.CompileError
	if errors.As(err, &one) {
		return []string{fmt.Sprintf("%s:%d %s", one.Source.Name, one.Pos.Line, one.Message)}
	}
	return nil
}

// a mistake in one declaration of a package does not hide those in the others
func TestCompileReportsEveryError(t *testing.T) {
	cases := []struct {
		files map[string]string
		want  []string // file:line and the start of each message, in order
	}{
		{map[string]string{"main.ev": `package main
const broken := nope
fn one() {
    if true {
        for x in [1] { return missing1 }
    }
}
fn two() => missing2
fn two() => 2
class A() : Ghost {}
class B(x) { fn m() => this.x + missing3 }
fn main() => 1`}, []string{
			"main.ev:2 cannot find 'nope'",
			"main.ev:5 cannot find 'missing1'",
			"main.ev:8 cannot find 'missing2'",
			"main.ev:9 'two' is declared twice",
			"main.ev:10 cannot find the parent class 'Ghost'",
			"main.ev:11 cannot find 'missing3'",
		}},
		// the compiler's state is put back after a mistake deep in a function,
		// so the next function compiles as if nothing happened
		{map[string]string{"main.ev": `package main
fn a() { while true { switch 1 { case 1: if true { x := (fn() => nope)() } } } }
fn b() { var n := 0
    for i in [1, 2] { n += i }
    return n }
fn main() => b()`}, []string{"main.ev:2 cannot find 'nope'"}},
		// errors from every file of a package, file by file
		{map[string]string{
			"main.ev":  "package main imports(\"./lib\")\nfn main() => lib.f()",
			"lib/a.ev": "package lib\nfn f() => g() + missingA",
			"lib/b.ev": "package lib\nfn g() => missingB",
		}, []string{"lib/a.ev:2 cannot find 'missingA'", "lib/b.ev:2 cannot find 'missingB'"}},
	}
	for _, c := range cases {
		var out bytes.Buffer
		got := compileErrors(checkFiles(t, c.files, &out))
		if len(got) != len(c.want) {
			t.Errorf("%v:\ngot  %q\nwant %q", c.files["main.ev"], got, c.want)
			continue
		}
		for i := range got {
			if !strings.HasPrefix(got[i], c.want[i]) {
				t.Errorf("error %d is %q, want %q", i+1, got[i], c.want[i])
			}
		}
	}
}

// a package with compile errors runs nothing, and there is a limit to how many
// errors one package reports
func TestCompileErrorsStopTheRun(t *testing.T) {
	var out bytes.Buffer
	eng := evie.New(evie.Options{Stdout: &out})
	defer eng.Close()
	_, err := eng.Eval("package main imports(\"io\")\nio.println(\"ran\")\nfn f() => nope\nfn main() => io.println(\"main ran\")")
	if err == nil || out.Len() != 0 {
		t.Fatalf("want a compile error and no output, got %v and %q", err, out.String())
	}

	var src strings.Builder
	src.WriteString("package main\n")
	for i := range 30 {
		fmt.Fprintf(&src, "fn f%d() => missing%d\n", i, i)
	}
	_, err = evie.New().Eval(src.String())
	if got := compileErrors(err); len(got) != 10 {
		t.Fatalf("want 10 errors, got %d: %v", len(got), err)
	}
	if !strings.Contains(err.Error(), "(and 9 more)") {
		t.Errorf("the one-line form should count the others: %q", err)
	}
}

// an engine that checked a program does not then run one
func TestCheckThenRun(t *testing.T) {
	b, err := evie.ParseSource("main.ev", []byte("package main\nfn main() => 1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := evie.Check(b); err != nil {
		t.Fatal(err)
	}
	eng := evie.New()
	defer eng.Close()
	if err := eng.Check(b); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.RunBundle(t.Context(), b); !errors.Is(err, evie.ErrRanAlready) {
		t.Fatalf("running after a check should be refused, got %v", err)
	}
}
