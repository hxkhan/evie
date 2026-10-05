package evie_test

import (
	"errors"
	"math"
	"os"
	"path/filepath"
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
	_, err := env.Eval(`package main
	fn direct() { return boom() }
	fn inTask() { return await go boom() }
	fn afterwards(a, b) { return a + b }
	`)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"direct", "inTask"} {
		fn, _ := env.Lookup("main." + name)
		_, exc := fn.Call()
		if exc == nil || !strings.Contains(exc.Error(), "internal error: host bug") {
			t.Fatalf("%s: expected the native's panic as an exception, got %v", name, exc)
		}
	}

	// the instance is still usable on the same fiber afterwards
	direct, _ := env.Lookup("main.direct")
	if _, exc := direct.Call(); exc == nil {
		t.Fatal("expected an exception")
	}
	afterwards, _ := env.Lookup("main.afterwards")
	res, exc := afterwards.Call(vm.BoxNumber(2), vm.BoxNumber(3))
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
	_, err := env.Eval(`package main
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
		if _, exc := spawn.Call(); exc != nil {
			t.Fatal(exc)
		}
		if _, exc := waiter.Call(); exc != nil {
			t.Fatal(exc)
		}
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
	_, err := env.Eval(`package main imports("time")
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
	res, exc := mainFn.Call()
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
			_, err := env.Eval(`package main
			fn fib(n) { if n < 2 { return n }
				return fib(n-1) + fib(n-2) }
			fn main() { return fib(20) }`)
			if err != nil {
				t.Error(err)
				results <- 0
				return
			}
			mainFn, _ := env.Lookup("main.main")
			res, exc := mainFn.Call()
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

// a receiver that has to be evaluated, such as a field, a subscript or a call
// result, takes the same path as a local: same results, receiver before
// arguments, same errors
func TestMemberCallsOnComputedReceivers(t *testing.T) {
	prelude := `package main imports("io")
	struct Box(v) {
		fn get() => this.v
		fn add(a, b) => this.v + a + b
	}
	struct Other(v) {
		fn get() => this.v * 100
	}
	struct Wrap(get) {
		fn get() => "method"
	}
	struct Plain(v) {}
	struct Holder(b) {
		fn viaField() => this.b.get()
	}
	var log := []
	fn note(tag, value) {
		log.push(tag)
		return value
	}
	fn make(v) => Box(v)
	var shared := Box(10)
	`
	for body, want := range map[string]string{
		`return Holder(Box(1)).viaField()`:                                                "1",
		`arr := [Box(2), Box(3)]; return arr[1].get() + arr[0].add(1, 2)`:                 "8",
		`return shared.get() + shared.add(1, 1)`:                                          "22",
		`return make(7).get()`:                                                            "7",
		`r := note("receiver", Box(1)).add(note("a", 1), note("b", 2)); return [r, log]`:  `[4, ["receiver", "a", "b"]]`,
		`h := {inner: {double: fn(x) => x * 2}}; return h.inner.double(21)`:               "42",
		`return ("ab" + "cd").toUpper()`:                                                  "ABCD",
		`o := {list: [1, 2]}; o.list.push(3); return o.list.len()`:                        "3",
		`return Holder(Wrap(fn() => "field")).b.get()`:                                    "field",
		`var t := 0; for s in [Box(1), Other(2), Box(3)] { t += [s][0].get() }; return t`: "204",
		`o := {pkg: io}; return o.pkg.println == io.println`:                              "true",
	} {
		val, err := runCode(prelude + "fn main() {\n" + strings.ReplaceAll(body, "; ", "\n") + "\n}")
		if err != nil {
			t.Errorf("%s: %v", body, err)
		} else if val.String() != want {
			t.Errorf("%s = %s, want %s", body, val, want)
		}
	}

	for body, want := range map[string]string{
		`return Holder(Box(1)).b.gte()`:   "no member 'gte' on a Box",
		`o := {x: nil}; return o.x.foo()`: "no member 'foo' on nil",
		`return Holder(Plain(1)).b.v()`:   "cannot call a number",
		`return [Box(1)][0].get(1, 2)`:    "takes no arguments but 2 were supplied",
		`return [[1]][0].pish(2)`:         "no member 'pish' on an array",
	} {
		_, err := runCode(prelude + "fn main() {\n" + strings.ReplaceAll(body, "; ", "\n") + "\n}")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want an error containing %q, got %v", body, want, err)
		}
	}
}

// a field can be assigned through any receiver, not only a named variable
func TestFieldAssignmentOnComputedReceivers(t *testing.T) {
	prelude := `package main
	struct P(v, child) {
		fn setChild(x) {
			this.child.v = x
		}
	}
	var log := []
	fn note(tag, value) {
		log.push(tag)
		return value
	}
	`
	for body, want := range map[string]string{
		`o := {a: {b: 1}}; o.a.b = 2; return o.a.b`:                                     "2",
		`arr := [{x: 1}]; arr[0].x = 5; return arr[0].x`:                                "5",
		`p := P(1, P(2, nil)); p.child.v = 3; return p.child.v`:                         "3",
		`p := P(1, P(2, nil)); p.setChild(4); return p.child.v`:                         "4",
		`o := {x: 1}; f := fn() => o; f().x = 8; return o.x`:                            "8",
		`p := P(1, P(2, nil)); p.child.extra = 9; return p.child.extra`:                 "9",
		`o := {a: {}}; note("receiver", o).a.b = note("value", 1); return [o.a.b, log]`: `[1, ["receiver", "value"]]`,
	} {
		val, err := runCode(prelude + "fn main() {\n" + strings.ReplaceAll(body, "; ", "\n") + "\n}")
		if err != nil {
			t.Errorf("%s: %v", body, err)
		} else if val.String() != want {
			t.Errorf("%s = %s, want %s", body, val, want)
		}
	}

	for body, want := range map[string]string{
		`o := {a: nil}; o.a.b = 1`:          "cannot assign field 'b' on nil",
		`o := {a: 5}; o.a.b = note("v", 1)`: "cannot assign field 'b' on number",
	} {
		_, err := runCode(prelude + "fn main() {\n" + strings.ReplaceAll(body, "; ", "\n") + "\n}")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want an error containing %q, got %v", body, want, err)
		}
	}

	// with the compiler's fast paths off, every receiver takes this path
	eng := evie.New(evie.Options{Debug: evie.Debug{NoFastPaths: true}})
	if _, err := eng.Load("noinline.ev", []byte("package main\nfn main() {\n\to := {a: 1}\n\to.a = 2\n\treturn o.a\n}")); err != nil {
		t.Fatal(err)
	}
	if val, err := eng.Call("main.main"); err != nil || val.String() != "2" {
		t.Fatalf("without fast paths: %v, %v", val, err)
	}
}

// an importer sees a package's vars as they are now, can assign them, and
// cannot assign its constants
func TestPackageVarsThroughImports(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("util.ev", "package util\nvar count := 0\nlimit := 5\nfn inc() {\n\tcount += 1\n}\nfn get() => count\n")
	write("main.ev", "package main imports(\"./util\")\nfn main() {\n\tutil.inc()\n\tutil.inc()\n\tbefore := util.count\n\tutil.count = 10\n\treturn [before, util.count, util.get(), util.limit]\n}\n")
	write("bad.ev", "package main imports(\"./util\")\nfn main() {\n\tutil.limit = 3\n}\n")

	val, err := evie.New().RunFile(filepath.Join(dir, "main.ev"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "[2, 10, 10, 5]"; val.String() != want {
		t.Fatalf("got %s, want %s", val, want)
	}

	_, err = evie.New().RunFile(filepath.Join(dir, "bad.ev"))
	if err == nil || !strings.Contains(err.Error(), "cannot assign to 'util.limit' because it is a constant") {
		t.Fatalf("assigning a package constant: %v", err)
	}
}

// compound assignment finds its variable by name: constants are reported as
// constants, and nothing depends on the compiler's fast paths
func TestCompoundAssignmentTargets(t *testing.T) {
	for src, want := range map[string]string{
		"package main\nlimit := 5\nfn main() {\n\tlimit += 1\n}":       "cannot assign to a constant",
		"package main\nfn main() {\n\tx := 1\n\tx += 1\n}":             "cannot assign to a constant",
		"package main\nfn f() => 1\nfn main() {\n\tf() += 1\n}":        "cannot assign to 'f()'",
		"package main\nfn main() {\n\tvar count := 0\n\tcuont += 1\n}": "cannot find 'cuont'",
	} {
		_, err := runCode(src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want an error containing %q, got %v", src, want, err)
		}
	}

	// the same programs run with the fast paths on and off
	src := `package main
	first := second + 1
	second := 10
	var total := 0
	fn main() {
		var x := 1
		x += 2
		x *= 3
		total += x
		total -= first
		return [x, total, first]
	}`
	for _, noInline := range []bool{false, true} {
		eng := evie.New(evie.Options{Debug: evie.Debug{NoFastPaths: noInline}})
		if _, err := eng.Load("compound.ev", []byte(src)); err != nil {
			t.Fatalf("NoInline %v: %v", noInline, err)
		}
		val, err := eng.Call("main.main")
		if err != nil {
			t.Fatalf("NoInline %v: %v", noInline, err)
		}
		if want := "[9, -2, 11]"; val.String() != want {
			t.Errorf("NoInline %v: got %s, want %s", noInline, val, want)
		}
	}
}

// calling a method on a computed receiver, or a variadic native, allocates
// nothing per call
func TestMemberCallsDoNotAllocate(t *testing.T) {
	eng := evie.New()
	_, err := eng.Load("alloc.ev", []byte(`package main imports("math")
	struct Box(v) {
		fn get() => this.v
	}
	struct Holder(b) {
		fn viaField() => this.b.get()
	}
	h := Holder(Box(1))
	items := [Box(2)]
	fn loop() {
		var total := 0
		var i := 0
		while i < 1000 {
			total += h.viaField() + items[0].get() + math.max(i, 7)
			i += 1
		}
		return total
	}`))
	if err != nil {
		t.Fatal(err)
	}
	loop, ok := eng.Lookup("main.loop")
	if !ok {
		t.Fatal("loop not found")
	}
	allocs := testing.AllocsPerRun(10, func() {
		if _, err := loop.Call(); err != nil {
			t.Fatal(err)
		}
	})
	// 3000 calls per run; a per-call allocation would show up as thousands
	if allocs > 20 {
		t.Fatalf("%.0f allocations for 3000 calls", allocs)
	}
}

// wrong argument counts at static call sites are compile errors that name the line
func TestStaticArityIsCompileTime(t *testing.T) {
	needsFastPaths(t)
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

	_, err := env.Eval(`package main
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
	if _, err := env.Eval("package main\nfn add(a, b) => a + b\nfn greet(who) => \"hi \" + who"); err != nil {
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

	_, err := env.Eval(`package main
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
	// a package is loaded once, so the addition goes in a package of its own
	if _, err := env.Eval("package extra\nfn peek(c) => c.get()"); err != nil {
		t.Fatal(err)
	}
	res, err = env.Call("extra.peek", &counter{n: 9})
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
	if _, err := evie.Eval("package main\nfn main() => undefinedName"); err == nil || !strings.Contains(err.Error(), "cannot find 'undefinedName'") {
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
