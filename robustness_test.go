package evie_test

import (
	"errors"
	"math"
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
		"f := fn() => 1\nreturn f.call()": "no member 'call' on a function",
		"f := fn() => 1\nreturn f.name":   "no member 'name' on a function",
		"return (5).foo()":                "no member 'foo' on a number",
		"return nil.foo":                  "no member 'foo' on nil",
		"x := 5\nreturn x()":              "cannot call a number",
		"return 5 + \"a\"":                "cannot apply '+' to a number and a string",
		"return [1, 2][5]":                "out of bounds",
		"return {}.missing":               "no member 'missing' on an object",
		"return \"abc\".len(1)":           "takes no arguments but 1 was supplied",
		"var o := {}\nreturn o.x.y":       "no member 'x' on an object",
		"return [1].map(5)":               "must be callable",
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
	env.SetStatic("boom", vm.BoxGoFunc(vm.NewFunc("boom", 0, func(c vm.Call) (vm.Value, error) {
		panic("host bug")
	})))
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
	env.SetStatic("probe", vm.BoxGoFunc(vm.NewFunc("probe", 0, func(c vm.Call) (vm.Value, error) {
		if inside.Add(1) != 1 {
			overlaps.Add(1)
		}
		for i := 0; i < 100; i++ {
			_ = i * i
		}
		inside.Add(-1)
		return vm.Value{}, nil
	})))
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
	if err == nil || !strings.Contains(err.Error(), "compile error") || !strings.Contains(err.Error(), ":4:") {
		t.Fatalf("expected a compile error naming line 4, got %v", err)
	}
}

// typed wrappers convert arguments and results without reflection and report
// type mismatches with the native's name
func TestTypedWrappers(t *testing.T) {
	env := evie.New()
	pkg := vm.NewPackage("m")
	pkg.Native("sqrt", vm.Func1(math.Sqrt))
	pkg.Native("hypot", vm.Func2(math.Hypot))
	pkg.Native("repeat", vm.Func2(strings.Repeat))
	pkg.Native("clamp", vm.Func3(func(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }))
	pkg.Native("isEven", vm.Func1(func(n int) bool { return n%2 == 0 }))
	pkg.Native("sum", vm.Func1(func(arr *vm.Array) float64 {
		var s float64
		for _, v := range arr.Data {
			f, _ := v.AsFloat64()
			s += f
		}
		return s
	}))
	pkg.Native("size", vm.Func1(func(o map[string]vm.Value) int { return len(o) }))
	pkg.Native("pair", vm.Func2(func(k string, v any) map[string]vm.Value {
		val, _ := vm.BoxAny(v)
		return map[string]vm.Value{k: val}
	}))
	pkg.Native("div", vm.Func2E(func(a, b float64) (float64, error) {
		if b == 0 {
			return 0, errors.New("division by zero")
		}
		return a / b, nil
	}))
	pkg.Native("now", vm.Func0(func() string { return "tick" }))
	env.SetStatic("m", pkg.Box())

	_, err := env.EvalString(`package main
	fn main() {
		r := [m.sqrt(16), m.hypot(3, 4), m.repeat("ab", 3), m.clamp(15, 0, 10), m.isEven(4), m.sum([1, 2, 3.5]), m.size({a: 1, b: 2}), m.pair("k", 7).k, m.now()]
		errs := [catch(m.div(1, 0)).message, catch(m.sqrt("x")).message, catch(m.isEven(1.5)).message]
		return [r, errs]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := env.Call("main.main")
	if err != nil {
		t.Fatal(err)
	}
	want := `[[4, 5, "ababab", 10, true, 6.5, 2, 7, "tick"], ["division by zero", "sqrt: argument 1 must be a number, got string", "isEven: argument 1 must be an integer, got number"]]`
	if res.String() != want {
		t.Fatalf("got  %s\nwant %s", res, want)
	}
}

// Instance.Call boxes Go arguments and borrows a fiber
func TestInstanceCall(t *testing.T) {
	env := evie.New()
	if _, err := env.EvalString("package main\nfn add(a, b) => a + b\nfn greet(who) => \"hi \" + who"); err != nil {
		t.Fatal(err)
	}
	sum, err := env.Call("main.add", 2, 3.5)
	if err != nil {
		t.Fatal(err)
	}
	if sum.String() != "5.5" {
		t.Fatalf("got %v", sum)
	}
	greeting, err := env.Call("main.greet", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if greeting.String() != "hi bob" {
		t.Fatalf("got %v", greeting)
	}
	if _, err := env.Call("main.missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a not-found error, got %v", err)
	}
	if _, err := env.Call("main.add", 1); err == nil || !strings.Contains(err.Error(), "takes 2 arguments but 1 was supplied") {
		t.Fatalf("expected an arity error, got %v", err)
	}
}

// host types travel through scripts as they are, with methods, identity, `is`
// checks, construction, and conversion in both directions
type counter struct{ n int }

func TestHostTypes(t *testing.T) {
	Counter := vm.NewHostType[*counter]("Counter")
	Counter.Constructor(1, func(c vm.Call) (vm.Value, error) {
		start, err := c.Int(0)
		if err != nil {
			return vm.Value{}, err
		}
		return Counter.Wrap(&counter{n: start}), nil
	})
	Counter.Method("inc", 0, func(c vm.Call) (vm.Value, error) {
		self, err := vm.HostArg[*counter](c, 0)
		if err != nil {
			return vm.Value{}, err
		}
		self.n++
		return c.This(), nil
	})
	Counter.Method("get", 0, func(c vm.Call) (vm.Value, error) {
		self, _ := vm.HostArg[*counter](c, 0)
		return vm.BoxNumber(float64(self.n)), nil
	})
	Counter.String = func(v any) string { return "Counter(" + vm.BoxNumber(float64(v.(*counter).n)).String() + ")" }

	env := evie.New()
	env.SetStatic("Counter", Counter.Box())
	shared := &counter{n: 40}
	env.SetStatic("shared", Counter.Wrap(shared))
	env.SetStatic("bump", vm.BoxGoFunc(vm.Func1(func(c *counter) *counter { c.n += 10; return c })))

	_, err := env.EvalString(`package main
	fn main() {
		c := Counter(5)
		c.inc()
		c.inc()
		shared.inc()
		same := bump(shared) == shared
		return [c.get(), shared.get(), string(c), c is Counter, 5 is Counter, same, catch(bump(3)).message]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := env.Call("main.main")
	if err != nil {
		t.Fatal(err)
	}
	want := `[7, 51, "Counter(7)", true, false, true, "native: argument 1 must be a Counter, got number"]`
	if res.String() != want {
		t.Fatalf("got  %s\nwant %s", res, want)
	}
	if shared.n != 51 {
		t.Fatalf("the host's own value was not mutated in place: %d", shared.n)
	}

	// a registered type boxes through BoxAny and Instance.Call as well
	res, err = env.Call("main.main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.EvalString("package main\nfn peek(c) => c.get()"); err != nil {
		t.Fatal(err)
	}
	res, err = env.Call("main.peek", &counter{n: 9})
	if err != nil {
		t.Fatal(err)
	}
	if res.String() != "9" {
		t.Fatalf("got %v", res)
	}
}

// top-level bindings are initialized in source order and may call functions
// and build any value; functions, structs and constants with constant
// initializers are hoisted
func TestPackageInitializers(t *testing.T) {
	code := `package main imports("io")
	var log := ""
	config := { debug: false, retries: 3 }
	limits := computeLimits(config.retries)
	first := second + 1
	second := 10
	p := Point(1, 2)
	log += "init;"

	fn computeLimits(n) => [n, n * 2]
	struct Point(x, y) { fn sum() => this.x + this.y }

	fn main() {
		config.debug = true
		return [config.debug, limits, first, second, p.sum(), log]
	}`
	val, err := runCode(code)
	if err != nil {
		t.Fatal(err)
	}
	want := `[true, [3, 6], 11, 10, 3, "init;"]`
	if val.String() != want {
		t.Fatalf("got %s, want %s", val, want)
	}

	// an initializer that throws fails the load with its message
	_, err = runCode("package main\nx := missing()\nfn missing() { throw error(\"no\") }\nfn main() { return x }")
	if err == nil || !strings.Contains(err.Error(), "no") {
		t.Fatalf("expected the initializer's error, got %v", err)
	}

	// a constant binding still cannot be reassigned, and a constant number has no fields
	_, err = runCode("package main\nx := 5\nfn main() { x = 6 }")
	if err == nil || !strings.Contains(err.Error(), "constant") {
		t.Fatalf("expected a constant-binding error, got %v", err)
	}
}

// the prelude is one package: builtins, host symbols, and anything the engine was given
func TestPrelude(t *testing.T) {
	prelude := vm.NewPackage("prelude")
	prelude.Set("answer", vm.BoxNumber(42))
	eng := evie.New(evie.Options{Prelude: prelude})
	eng.SetStatic("greeting", vm.BoxString("hi"))
	if _, err := eng.Eval("package main\nfn main() => [answer, greeting, string(answer) is string]"); err != nil {
		t.Fatal(err)
	}
	val, err := eng.Call("main.main")
	if err != nil {
		t.Fatal(err)
	}
	if val.String() != `[42, "hi", true]` {
		t.Fatalf("got %s", val)
	}
	if v, ok := eng.Lookup("answer"); !ok || v.String() != "42" {
		t.Fatalf("Lookup through the prelude failed: %v %v", v, ok)
	}
	if _, err := evie.Run("package main\nfn main() => undefinedName"); err == nil || !strings.Contains(err.Error(), "cannot find 'undefinedName'") {
		t.Fatalf("expected an undefined symbol error, got %v", err)
	}
}

// host packages are imported by name and resolved before the filesystem
func TestEngineOptions(t *testing.T) {
	maths := vm.NewPackage("maths")
	maths.Native("twice", vm.Func1(func(x float64) float64 { return x * 2 }))
	resolved := 0
	eng := evie.New(evie.Options{
		Packages: []*vm.Package{maths},
		Resolve: func(path string) (*vm.Package, error) {
			resolved++
			if path == "dyn" {
				p := vm.NewPackage("dyn")
				p.Set("k", vm.BoxNumber(7))
				return p, nil
			}
			return nil, nil
		},
		NoStdlib: true,
	})
	pkg, err := eng.Load("main.ev", []byte("package main imports(\"maths\", \"dyn\")\nfn main() => maths.twice(dyn.k)"))
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name() != "main" {
		t.Fatalf("Load returned %q", pkg.Name())
	}
	val, err := eng.Call("main.main")
	if err != nil {
		t.Fatal(err)
	}
	if val.String() != "14" || resolved != 1 {
		t.Fatalf("got %s after %d resolver calls", val, resolved)
	}
	if _, err := eng.Eval("package other imports(\"io\")"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected the standard library to be unavailable, got %v", err)
	}
}
