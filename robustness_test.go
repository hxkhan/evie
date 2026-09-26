package evie_test

import (
	"strings"
	"testing"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

// a script can be wrong in every way it likes; the host process must survive it
func TestRuntimeErrorsNeverPanic(t *testing.T) {
	cases := map[string]string{
		"x := 5\nx.foo = 1\nreturn x":     "cannot assign field 'foo' on number",
		"f := fn() => 1\nreturn f.call()": "undefined symbol 'call'",
		"f := fn() => 1\nreturn f.name":   "undefined symbol 'name'",
		"return (5).foo()":                "undefined symbol 'foo'",
		"return nil.foo":                  "undefined symbol 'foo'",
		"x := 5\nreturn x()":              "cannot call a non-function",
		"return 5 + \"a\"":                "cannot apply '+' operator",
		"return [1, 2][5]":                "out of bounds",
		"return {}.missing":               "undefined symbol 'missing'",
		"return \"abc\".len(1)":           "argument(s)",
		"var o := {}\nreturn o.x.y":       "undefined symbol",
		"return [1].map(5)":               "cannot invoke a non-callable",
		"s := \"x\"\nreturn s[\"k\"]":     "string index must be a number",
	}
	for code, want := range cases {
		_, err := runCode(code)
		if err == nil {
			t.Errorf("%q: expected an error mentioning %q, got none", code, want)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %q, want it to mention %q", code, err, want)
		}
	}
}

// a panicking native is a bug in the host, which is reported, never propagated
func TestPanickingNativeBecomesException(t *testing.T) {
	env := evie.New()
	env.SetStatic("boom", vm.BoxGoFunc(&vm.GoFunc{Name: "boom", Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		panic("host bug")
	}}))
	_, err := env.EvalString(`package main
	fn direct() { return boom() }
	fn inTask() { return await go boom() }
	fn afterwards(a, b) { return a + b }
	`)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"direct", "inTask"} {
		fn, _ := env.Lookup("main." + name)
		_, exc := fn.Call(nil)
		if exc == nil || !strings.Contains(exc.Error(), "internal error: host bug") {
			t.Fatalf("%s: expected the native's panic as an exception, got %v", name, exc)
		}
	}

	// the instance is still usable on the same fiber afterwards
	fbr := env.NewFiber()
	defer env.ReleaseFiber(fbr)
	direct, _ := env.Lookup("main.direct")
	if _, exc := direct.Call(fbr); exc == nil {
		t.Fatal("expected an exception")
	}
	afterwards, _ := env.Lookup("main.afterwards")
	res, exc := afterwards.Call(fbr, vm.BoxNumber(2), vm.BoxNumber(3))
	if exc != nil {
		t.Fatal(exc)
	}
	if n, _ := res.AsFloat64(); n != 5 {
		t.Fatalf("expected 5, got %v", res)
	}
}

// a fiber that ran tasks is handed out clean afterwards
func TestFibersAreResetBetweenUses(t *testing.T) {
	env := evie.New()
	_, err := env.EvalString(`package main
	fn s() { return 1 }
	fn spawn() { var i := 0
		while i < 50 { await go s()
			i += 1 } }
	fn waiter() { return await go s() }
	`)
	if err != nil {
		t.Fatal(err)
	}
	spawn, _ := env.Lookup("main.spawn")
	waiter, _ := env.Lookup("main.waiter")
	for i := 0; i < 200; i++ {
		if _, exc := spawn.Call(nil); exc != nil {
			t.Fatal(exc)
		}
		fbr := env.NewFiber()
		if _, exc := waiter.Call(fbr); exc != nil {
			t.Fatal(exc)
		}
		env.ReleaseFiber(fbr)
	}
}

// a function stored in an object field is called as a plain function
func TestFunctionsInObjectFieldsAreNotMethods(t *testing.T) {
	cases := map[string]string{
		"fn id(this) => this\nfn main() { o := { f: id }\n return o.f(7) }":                     "7",
		"fn main() { o := { f: fn(a, b) => a + b }\n return o.f(1, 2) }":                        "3",
		"fn main() { o := { s: string }\n return o.s(42) }":                                     "42",
		"fn main() { o := { k: {}.keys }\n return o.k().len() }":                                "0",
		"struct P(n) { fn get() => this.n }\nfn main() { o := { g: P(5).get }\n return o.g() }": "5",
	}
	for code, want := range cases {
		val, err := runCode("package main\n" + code)
		if err != nil {
			t.Errorf("%q: %v", code, err)
			continue
		}
		if val.String() != want {
			t.Errorf("%q: got %v, want %v", code, val, want)
		}
	}
}

// object literal fields are evaluated in the order they are written
func TestObjectLiteralEvaluationOrder(t *testing.T) {
	code := `package main
	var log := ""
	fn tap(x) { log += string(x)
		return x }
	fn main() {
		o := { c: tap(3), a: tap(1), b: tap(2), d: tap(4) }
		return log + ":" + string(o.a + o.b + o.c + o.d)
	}`
	for i := 0; i < 20; i++ {
		val, err := runCode(code)
		if err != nil {
			t.Fatal(err)
		}
		if val.String() != "3124:10" {
			t.Fatalf("got %v, want 3124:10", val)
		}
	}
}
