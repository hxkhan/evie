package evie_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

// Equals and IsTruthy agree with the language
func TestValuePredicates(t *testing.T) {
	n := vm.BoxNumber
	s := vm.BoxString

	if !n(1).Equals(n(1)) || n(0).IsTruthy() || !s("x").IsTruthy() {
		t.Fatal("Equals or IsTruthy disagree with the language")
	}
}

// intOf, floatOf and boolOf read a number or a bool, giving zero otherwise
func intOf(v vm.Value) int {
	f, _ := v.AsFloat64()
	return int(f)
}

func floatOf(v vm.Value) float64 {
	f, _ := v.AsFloat64()
	return f
}

func boolOf(v vm.Value) bool {
	b, _ := v.AsBool()
	return b
}

// hosts read and write fields of objects and class instances by name
func TestPublicFields(t *testing.T) {
	eng := evie.New()
	if _, err := eng.Load("shapes.ev", []byte("package shapes\nclass Point(x, y) {\n\tvar label := \"\"\n\ttag := \"pt\"\n}\nfn make() => Point(1, 2)\nfn obj() => {a: 1}")); err != nil {
		t.Fatal(err)
	}
	p, err := eng.Call("shapes.make")
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := p.ClassName(); !ok || name != "Point" {
		t.Fatalf("ClassName = %q, %v", name, ok)
	}
	if x, ok := p.Field("x"); !ok || x.String() != "1" {
		t.Fatalf("Field(x) = %v, %v", x, ok)
	}
	if err := p.SetField("y", vm.BoxNumber(5)); err != nil {
		t.Fatal(err)
	}
	if err := p.SetField("label", vm.BoxString("origin")); err != nil {
		t.Fatal(err)
	}
	if err := p.SetField("unknown", vm.BoxString("nope")); err == nil || !strings.Contains(err.Error(), "has no field 'unknown'") {
		t.Fatalf("SetField on undeclared field should fail, got: %v", err)
	}
	if err := p.SetField("tag", vm.BoxString("other")); err == nil || !strings.Contains(err.Error(), "cannot assign to constant field 'tag'") {
		t.Fatalf("SetField on constant field should fail, got: %v", err)
	}
	if y, _ := p.Field("y"); y.String() != "5" {
		t.Fatalf("Field(y) after SetField = %v", y)
	}
	if l, ok := p.Field("label"); !ok || l.String() != "origin" {
		t.Fatalf("body field = %v, %v", l, ok)
	}
	o, _ := eng.Call("shapes.obj")
	if a, ok := o.Field("a"); !ok || a.String() != "1" {
		t.Fatalf("object Field(a) = %v, %v", a, ok)
	}
	if err := vm.BoxNumber(1).SetField("x", vm.BoxNumber(2)); err == nil {
		t.Fatal("SetField on a number should fail")
	}
}

// a looked-up function is called from Go without fibers, and errors are plain errors
func TestPublicCallsAndTasks(t *testing.T) {
	slow := vm.NewPackage("slow")
	slow.Func("double", 1, func(c vm.Call) (vm.Value, error) {
		n, err := c.Number(0)
		if err != nil {
			return vm.Value{}, err
		}
		return c.Async(func(ctx context.Context) (vm.Value, error) {
			if n < 0 {
				return vm.Value{}, errors.New("negative")
			}
			return vm.BoxNumber(n * 2), nil
		}), nil
	})
	eng := evie.New(evie.Options{Packages: []*vm.Package{slow}})
	defer eng.Close()
	if _, err := eng.Load("main.ev", []byte("package main imports(\"slow\")\nfn run(n) => await slow.double(n)\nfn start(n) => slow.double(n)\nfn bad() => 1 + \"x\"")); err != nil {
		t.Fatal(err)
	}
	run, _ := eng.Lookup("main.run")
	if v, err := run.Call(vm.BoxNumber(21)); err != nil || v.String() != "42" {
		t.Fatalf("run(21) = %v, %v", v, err)
	}
	if _, err := run.Call(vm.BoxNumber(-1)); err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("run(-1) gave %v", err)
	}

	bad, _ := eng.Lookup("main.bad")
	_, err := bad.Call()
	if err == nil {
		t.Fatal("bad() should fail")
	}
	if out := evie.Format(err, false); !strings.Contains(out, "cannot apply '+'") || !strings.Contains(out, "--> main.ev:4:") {
		t.Fatalf("Format did not render the report:\n%s", out)
	}
	if out := evie.Format(errors.New("plain"), false); out != "error: plain\n" {
		t.Fatalf("Format of a plain error = %q", out)
	}

	start, _ := eng.Lookup("main.start")
	task, err := start.Call(vm.BoxNumber(7))
	if err != nil {
		t.Fatal(err)
	}
	tk, ok := task.AsTask()
	if !ok {
		t.Fatal("Call.Async did not return a task")
	}
	<-tk.Done()
	if v, err := tk.Result(); err != nil || v.String() != "14" {
		t.Fatalf("Result = %v, %v", v, err)
	}
}

func TestDumpAndLoadWithoutRunningMain(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.ev")
	src := `package main imports("math")

const PI := 3.14
var hits := 0

class Base(x) {
	fn get() => this.x
}

class Child(x, y) : Base {
	fn sum() => super.get() + this.y
}

fn outer(seed) {
	var count := seed
	class LocalBox(v) {
		fn bumped() => this.v + count
	}
	return fn() {
		return fn() => count
	}
}

fn main() {
	throw error("main must not run during LoadFile")
}
`
	if err := os.WriteFile(mainPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	eng := evie.New(evie.Options{Debug: evie.Debug{DebugInfo: true}})
	if err := loadFile(eng, mainPath); err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	var buf bytes.Buffer
	eng.Dump(&buf)
	out := buf.String()

	for _, want := range []string{
		"package main",
		"imports: math",
		"const PI = 3.14 [folded]",
		"var hits [runtime]",
		"class Base",
		"class Child : Base",
		"fields: [0] x, [1] y",
		"fn outer(seed)",
		"[1] count (var, escapes)",
		"class LocalBox",
		"[0] <- Local(1) 'count'",
		"[0] <- Captured(0) 'count'",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Dump output missing %q:\n%s", want, out)
		}
	}

	// Bare script compiles without executing side effects in LoadFile
	barePath := filepath.Join(dir, "bare.ev")
	if err := os.WriteFile(barePath, []byte("var x := 1\nthrow error(\"must not run\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bareEng := evie.New(evie.Options{Debug: evie.Debug{DebugInfo: true}})
	if err := loadFile(bareEng, barePath); err != nil {
		t.Fatalf("LoadFile on bare script failed: %v", err)
	}
	var bareBuf bytes.Buffer
	bareEng.Dump(&bareBuf)
	if !strings.Contains(bareBuf.String(), "[0] x (var)") {
		t.Fatalf("bare script dump missing local slot:\n%s", bareBuf.String())
	}

	// Dump also works gracefully when DebugInfo is false (default zero-overhead mode)
	plainEng := evie.New()
	if _, err := plainEng.Load("plain.ev", []byte("package main\nfn add(a, b) => a + b\n")); err != nil {
		t.Fatal(err)
	}
	var plainBuf bytes.Buffer
	plainEng.Dump(&plainBuf)
	if !strings.Contains(plainBuf.String(), "fn add(a, b)") {
		t.Fatalf("Dump without DebugInfo missing function:\n%s", plainBuf.String())
	}
}

// loadFile compiles a file and its imports into eng without running main
func loadFile(eng *evie.Engine, path string) error {
	b, err := evie.ParseFile(nil, path)
	if err != nil {
		return err
	}
	return eng.LoadBundle(context.Background(), b)
}
