package evie_test

import (
	"strings"
	"sync/atomic"
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

// fibers of one instance never execute evie code at the same time; a native
// counts how many fibers are inside it and fails the moment it sees a second
func TestOneFiberRunsAtATime(t *testing.T) {
	env := evie.New()
	var inside, overlaps atomic.Int32
	env.SetStatic("probe", vm.BoxGoFunc(&vm.GoFunc{Name: "probe", Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		if inside.Add(1) != 1 {
			overlaps.Add(1)
		}
		for i := 0; i < 100; i++ {
			_ = i * i
		}
		inside.Add(-1)
		return vm.Value{}, nil
	}}))
	_, err := env.EvalString(`package main imports("time")
	var pushes := 0
	fn worker(id, n) {
		var i := 0
		while i < n {
			probe()
			pushes += 1
			if i % 50 == 0 { await go probe() }
			i += 1
		}
	}
	fn sleeper() { time.wait(5) }
	fn main() {
		tasks := [go worker(1, 2000), go worker(2, 2000), go worker(3, 2000), go worker(4, 2000), go sleeper()]
		await.all(tasks)
		return pushes
	}
	`)
	if err != nil {
		t.Fatal(err)
	}
	mainFn, _ := env.Lookup("main.main")
	res, exc := mainFn.Call(nil)
	if exc != nil {
		t.Fatal(exc)
	}
	if n, _ := res.AsFloat64(); n != 8000 {
		t.Fatalf("expected 8000 pushes, got %v", res)
	}
	if overlaps.Load() != 0 {
		t.Fatalf("%d overlapping entries into a native from different fibers", overlaps.Load())
	}
}

// several instances run in parallel without sharing anything
func TestInstancesRunInParallel(t *testing.T) {
	const instances = 4
	results := make(chan float64, instances)
	for i := 0; i < instances; i++ {
		go func() {
			env := evie.New()
			_, err := env.EvalString(`package main
			fn fib(n) { if n < 2 { return n }
				return fib(n-1) + fib(n-2) }
			fn main() { return fib(20) }`)
			if err != nil {
				t.Error(err)
				results <- 0
				return
			}
			mainFn, _ := env.Lookup("main.main")
			res, exc := mainFn.Call(nil)
			if exc != nil {
				t.Error(exc)
			}
			n, _ := res.AsFloat64()
			results <- n
		}()
	}
	for i := 0; i < instances; i++ {
		if n := <-results; n != 6765 {
			t.Fatalf("expected 6765, got %v", n)
		}
	}
}

// calling through a local receiver: fields holding callables, methods through
// inheritance, and a call site that sees several struct types in turn
func TestMemberCalls(t *testing.T) {
	code := `package main
	struct Animal(name) {
		fn speak() => this.name + " makes a sound"
	}
	struct Dog(name, trick) : Animal {
		fn speak() => this.name + " barks"
	}
	struct Holder(cb) {}
	fn main() {
		animals := [Animal("Generic"), Dog("Rex", fn(x) => x * 2), Animal("Other"), Dog("Fido", fn(x) => x + 1)]
		var out := ""
		for a in animals {
			out += a.speak() + ";"
		}
		h := Holder(fn(a, b) => a - b)
		out += string(h.cb(10, 4)) + ";"
		d := animals[1]
		out += string(d.trick(21)) + ";"
		d.extra = fn() => "dynamic"
		out += d.extra()
		return out
	}`
	val, err := runCode(code)
	if err != nil {
		t.Fatal(err)
	}
	want := "Generic makes a sound;Rex barks;Other makes a sound;Fido barks;6;42;dynamic"
	if val.String() != want {
		t.Fatalf("got %q, want %q", val, want)
	}
}

// wrong argument counts at static call sites are compile errors that name the line
func TestStaticArityIsCompileTime(t *testing.T) {
	_, err := runCode("package main\nfn add(a, b) => a + b\nfn main() {\n\treturn add(1)\n}")
	if err == nil || !strings.Contains(err.Error(), "compile error") || !strings.Contains(err.Error(), "line 4") {
		t.Fatalf("expected a compile error naming line 4, got %v", err)
	}
}
