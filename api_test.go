package evie_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

// the public operators behave like the script's, with the script's errors
func TestPublicOperators(t *testing.T) {
	n := vm.BoxNumber
	s := vm.BoxString

	if v, err := n(2).Add(n(3)); err != nil || v.String() != "5" {
		t.Fatalf("2 + 3 = %v, %v", v, err)
	}
	if v, err := s("ab").Add(s("cd")); err != nil || v.String() != "abcd" {
		t.Fatalf(`"ab" + "cd" = %v, %v`, v, err)
	}
	if v, err := n(7).Mod(n(4)); err != nil || v.String() != "3" {
		t.Fatalf("7 %% 4 = %v, %v", v, err)
	}
	if ok, err := n(1).LessThan(n(2)); err != nil || !ok {
		t.Fatalf("1 < 2 = %v, %v", ok, err)
	}
	if ok, err := n(2).GreaterThanOrEqualTo(n(2)); err != nil || !ok {
		t.Fatalf("2 >= 2 = %v, %v", ok, err)
	}
	if _, err := n(1).Add(s("x")); err == nil || !strings.Contains(err.Error(), "cannot apply '+' to a number and a string") {
		t.Fatalf("1 + \"x\" gave %v", err)
	}
	if _, err := s("a").LessThan(s("b")); err == nil || !strings.Contains(err.Error(), "cannot apply '<'") {
		t.Fatalf(`"a" < "b" gave %v`, err)
	}
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

// hosts read and write fields of objects and struct instances by name
func TestPublicFields(t *testing.T) {
	eng := evie.New()
	if _, err := eng.Load("shapes.ev", []byte("package shapes\nstruct Point(x, y) {}\nfn make() => Point(1, 2)\nfn obj() => {a: 1}")); err != nil {
		t.Fatal(err)
	}
	p, err := eng.Call("shapes.make")
	if err != nil {
		t.Fatal(err)
	}
	if name, ok := p.StructName(); !ok || name != "Point" {
		t.Fatalf("StructName = %q, %v", name, ok)
	}
	if x, ok := p.Field("x"); !ok || x.String() != "1" {
		t.Fatalf("Field(x) = %v, %v", x, ok)
	}
	if err := p.SetField("y", vm.BoxNumber(5)); err != nil {
		t.Fatal(err)
	}
	if err := p.SetField("label", vm.BoxString("dynamic")); err != nil {
		t.Fatal(err)
	}
	if y, _ := p.Field("y"); y.String() != "5" {
		t.Fatalf("Field(y) after SetField = %v", y)
	}
	if l, ok := p.Field("label"); !ok || l.String() != "dynamic" {
		t.Fatalf("dynamic field = %v, %v", l, ok)
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
		return vm.Async(func() (vm.Value, error) {
			if n < 0 {
				return vm.Value{}, errors.New("negative")
			}
			return vm.BoxNumber(n * 2), nil
		}), nil
	})
	eng := evie.New(evie.Options{Packages: []*vm.Package{slow}})
	if _, err := eng.Load("main.ev", []byte("package main imports(\"slow\")\nfn run(n) => await slow.double(n)\nfn bad() => 1 + \"x\"")); err != nil {
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
	if out := evie.Format(err, false); !strings.Contains(out, "cannot apply '+'") || !strings.Contains(out, "--> main.ev:3:") {
		t.Fatalf("Format did not render the report:\n%s", out)
	}
	if out := evie.Format(errors.New("plain"), false); out != "error: plain\n" {
		t.Fatalf("Format of a plain error = %q", out)
	}

	task := vm.Async(func() (vm.Value, error) { return vm.BoxString("done"), nil })
	tk, ok := task.AsTask()
	if !ok {
		t.Fatal("Async did not return a task")
	}
	<-tk.Done()
	if v, err := tk.Result(); err != nil || v.String() != "done" {
		t.Fatalf("Result = %v, %v", v, err)
	}
}
