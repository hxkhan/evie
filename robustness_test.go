package evie_test

import (
	"context"
	"errors"
	"fmt"
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
		"fn id(this) => this\nfn main() { o := { f: id }\n return o.f(7) }":                    "7",
		"fn main() { o := { f: fn(a, b) => a + b }\n return o.f(1, 2) }":                       "3",
		"fn main() { o := { s: string }\n return o.s(42) }":                                    "42",
		"fn main() { o := { k: {}.keys }\n return o.k().len() }":                               "0",
		"class P(n) { fn get() => this.n }\nfn main() { o := { g: P(5).get }\n return o.g() }": "5",
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
// a host can call into an engine while another goroutine loads packages into
// it and sets statics; run with -race to see the guarantee
func TestCallsWhileLoading(t *testing.T) {
	eng := evie.New()
	defer eng.Close()
	if _, err := eng.Eval("package main\nvar ticks := 0\nfn tick() { ticks += 1; return ticks }"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 50 {
			name := "p" + string(rune('a'+i%26)) + string(rune('a'+i/26))
			if _, err := eng.Load(name+".ev", []byte("package "+name+"\nfn f() => 1\nx := f() + 1\n")); err != nil {
				t.Error(err)
				return
			}
			eng.SetStatic("s"+name, vm.BoxNumber(float64(i)))
		}
	}()
	calls := 0
	for {
		select {
		case <-done:
			if calls == 0 {
				t.Fatal("no call ran while the packages loaded")
			}
			return
		default:
		}
		if _, err := eng.Call("main.tick"); err != nil {
			t.Fatal(err)
		}
		if _, ok := eng.Lookup("sabc"); ok {
			t.Fatal("found a static that was never set")
		}
		calls++
	}
}

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
// inheritance, and a call site that sees several class types in turn
func TestMemberCalls(t *testing.T) {
	code := `package main
	class Animal(name) {
		fn speak() => this.name + " makes a sound"
	}
	class Dog(name, trick) : Animal {
		var extra := nil
		fn speak() => this.name + " barks"
	}
	class Holder(cb) {}
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
	class Box(v) {
		fn get() => this.v
		fn add(a, b) => this.v + a + b
	}
	class Other(v) {
		fn get() => this.v * 100
	}
	class Wrap(get) : Box {}
	class Plain(v) {}
	class Holder(b) {
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
	class P(v, child) {
		var extra := 0
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
		`o := {a: nil}; o.a.b = 1`:               "cannot assign field 'b' on nil",
		`o := {a: 5}; o.a.b = note("v", 1)`:      "cannot assign field 'b' on number",
		`p := P(1, P(2, nil)); p.child.nope = 9`: "class 'P' has no field 'nope'",
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
	class Box(v) {
		fn get() => this.v
	}
	class Holder(b) {
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
	pkg.Native("size", vm.Func1(func(o *vm.Object) int { return o.Len() }))
	pkg.Native("pair", vm.Func2(func(k string, v any) *vm.Object {
		val, _ := vm.BoxAny(v)
		o := vm.NewObject(1)
		o.Set(k, val)
		return o
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

// a native called with Value.Call runs outside any instance and cannot start
// async work; CallValue runs it under the engine, where it can
func TestCallValueRunsNativesUnderTheEngine(t *testing.T) {
	later := vm.NewFunc("later", 0, func(c vm.Call) (vm.Value, error) {
		return c.Async(func(ctx context.Context) (vm.Value, error) { return vm.BoxNumber(7), nil }), nil
	})
	native := vm.BoxGoFunc(later)

	detached, err := native.Call()
	if err != nil {
		t.Fatal(err)
	}
	task, _ := detached.AsTask()
	<-task.Done()
	if _, err := task.Result(); err == nil || !strings.Contains(err.Error(), "needs an instance") {
		t.Fatalf("detached async gave %v", err)
	}

	eng := evie.New()
	defer eng.Close()
	v, err := eng.CallValue(native)
	if err != nil {
		t.Fatal(err)
	}
	task, _ = v.AsTask()
	<-task.Done()
	if res, err := task.Result(); err != nil || res.String() != "7" {
		t.Fatalf("async under the engine gave %v, %v", res, err)
	}
	if _, err := eng.CallValue(vm.BoxNumber(1)); err == nil {
		t.Fatal("calling a number should fail")
	}
}

// registering a Go type again gives the type registered first, so a value boxed
// by one engine is the same type in another; another name for it panics
type token struct{ id int }

func TestHostTypeRegisteredTwice(t *testing.T) {
	first := vm.NewHostType[*token]("Token")
	second := vm.NewHostType[*token]("Token")
	if first != second {
		t.Fatal("registering a type again made a second one")
	}
	boxed, err := vm.BoxAny(&token{id: 1})
	if err != nil {
		t.Fatal(err)
	}
	eng := evie.New()
	eng.SetStatic("Token", first.Box())
	eng.SetStatic("tok", boxed)
	if v, err := eng.Eval("return tok is Token"); err != nil || v.String() != "true" {
		t.Fatalf("tok is Token = %v, %v", v, err)
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "already registered as 'Token'") {
			t.Fatalf("expected a panic for another name, got %v", r)
		}
	}()
	vm.NewHostType[*token]("Other")
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
// and build any value; functions, classes and constants with constant
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
	class Point(x, y) { fn sum() => this.x + this.y }

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

// nested named functions bind in local scope, support recursion, and work in switch cases
func TestNestedNamedFunctions(t *testing.T) {
	code := `package main
	fn main() {
		fn addOne(x) => x + 1
		fn fact(n) {
			if n <= 1 { return 1 }
			return n * fact(n - 1)
		}
		var swRes := 0
		switch 1 {
		case 1:
			fn double(v) => v * 2
			swRes = double(21)
		}
		return [addOne(5), fact(5), swRes]
	}`
	val := run(t, code)
	if want := "[6, 120, 42]"; val.String() != want {
		t.Fatalf("got %s, want %s", val, want)
	}
}

// an initializer sees the bindings from before its declaration, never its own
// unwritten slot; only a function literal naming the binding sees it
func TestDeclarationInitializersSeeOuterBindings(t *testing.T) {
	val := run(t, `package main
	N := 4
	fn outerConst() { N := N * 2; return N }
	fn wrap(f) => f
	fn main() {
		y := 1
		var inner := 0
		if true { y := y + 10; inner = y }
		x := 5
		f := fn() { x := x + 1; return x }
		fact := fn(n) { if n < 2 { return 1 }; return n * fact(n - 1) }
		g := wrap(fn(n) { if n < 1 { return 0 }; return 1 + g(n - 1) })
		return [y, inner, f(), outerConst(), fact(5), g(4)]
	}`)
	if want := "[1, 11, 6, 8, 120, 4]"; val.String() != want {
		t.Fatalf("got %s, want %s", val, want)
	}

	// `v := v` used to read whatever the previous call left in the slot: a stale
	// string, or a heap cell reinterpreted as a string, which crashed the process
	_, err := runCode(`package main
	fn leak() { c := 1.5; f := fn() => c; return f }
	fn peek() { v := v; return v }
	fn main() { g := leak(); x := peek(); return x.len() }`)
	if err == nil || !strings.Contains(err.Error(), "cannot find 'v'") {
		t.Fatalf("expected a compile error for a self-referencing declaration, got %v", err)
	}
}

// a parameter of a nested function shadows outer bindings, so an outer local of
// the same name stays on the stack and keeps its loop fast paths
func TestParametersDoNotMakeOuterLocalsEscape(t *testing.T) {
	val := run(t, `package main
	fn main() {
		var x := 0
		while x < 10 { x += 1 }
		ys := [1, 2].map(fn(x) => x * 2)
		var i := 0
		while i < 10 { d := 3; i += d }
		return [x, ys, i]
	}`)
	if want := "[10, [2, 4], 12]"; val.String() != want {
		t.Fatalf("got %s, want %s", val, want)
	}
}

// every await of a failed task gets its own copy of the error, so traces do
// not pile up, and a cancelled task's error is an ordinary error when thrown
func TestTaskErrorsAreCopiedForEachAwait(t *testing.T) {
	val := run(t, `package main imports("time")
	fn fail() { throw error("boom") }
	fn main() {
		t := go fail()
		a := catch(await t)
		n := a.trace.len()
		b := catch(await t)
		c := go time.sleep(100000)
		c.cancel()
		catch(await c)
		e := c.error()
		r := catch { throw e }
		return [a.trace.len() == n, a != b, r != nil, r.message]
	}`)
	if want := `[true, true, true, "cancelled"]`; val.String() != want {
		t.Fatalf("got %s, want %s", val, want)
	}
}

// objects keep their keys in the order they were added; the objects a literal
// makes share its keys until one changes, and big objects switch to an index
func TestObjectLayout(t *testing.T) {
	val := run(t, `package main
	fn make() => {x: 1, y: 2, x: 3}
	fn main() {
		a := make()
		b := make()
		a.z = 4
		b.remove("x")

		var big := {}
		var i := 0
		while i < 12 { big["k" + string(i)] = i; i += 1 }
		big.remove("k3")

		var seen := []
		var o := {p: 1, q: 2, r: 3}
		for k, v in o {
			if k == "p" { o.remove("q") }
			seen.push(k + "=" + string(v))
		}

		s := "xy"
		var d := {}
		d[s[0:1]] = "short"
		d[s] = "long"

		return [a, b, a.keys(), b.keys(), big.len(), big.k11, big.has("k3"), seen, d.x, d.xy, d[s[0:1]]]
	}`)
	want := `[{x: 3, y: 2, z: 4}, {y: 2}, ["x", "y", "z"], ["y"], 11, 11, false, ["p=1", "r=3"], "short", "long", "short"]`
	if val.String() != want {
		t.Fatalf("got  %s\nwant %s", val, want)
	}
}

// compound updates of fields and elements take an inlined path for numbers and
// keep every other behaviour: strings, negative indexes, %, and type errors
func TestCompoundUpdatesOfFieldsAndElements(t *testing.T) {
	val := run(t, `package main
	class C() {
		var n := 1
		var s := "a"
		fn bump() { this.n += 2; this.n *= 3; this.n %= 5; this.s += "b"; return [this.n, this.s] }
	}
	fn main() {
		c := C()
		r := c.bump()
		var arr := [1, 2, "x"]
		var i := 0
		var k := 2
		arr[i] += 10
		arr[k] += "y"
		var neg := -1
		arr[neg] += "z"
		var o := { a: 1, b: "p" }
		o.a -= 3
		o.b += "q"
		c.n /= 2
		var half := 0.5
		return [r, arr, o.a, o.b, c.n, catch { arr[half] += 1 }.message, catch { c.s -= 1 }.message]
	}`)
	want := `[[4, "ab"], [11, 2, "xyz"], -2, "pq", 2, "array index must be an integer, got 0.5", "cannot apply '-' to a string and a number"]`
	if val.String() != want {
		t.Fatalf("got  %s\nwant %s", val, want)
	}
}

// a '(' or '[' at the start of a line begins a new statement instead of calling
// or indexing the line before; a '.' still continues it
func TestLineBreakEndsCallsAndIndexes(t *testing.T) {
	val := run(t, `package main
	fn main() {
		arr := [5, 6]
		x := arr
		[1, 2].len()
		f := fn(v) => v
		y := f
		(3)
		return [x, y(4), arr
			.len()]
	}`)
	if want := "[[5, 6], 4, 2]"; val.String() != want {
		t.Fatalf("got %s, want %s", val, want)
	}
}

// closures and tasks inside for and while loops capture per-iteration bindings
func TestLoopClosureAndTaskCaptures(t *testing.T) {
	code := `package main
	fn main() {
		var fns := []
		for i, x in [10, 20, 30] {
			y := x + i
			fn localFn() => [i, x, y]
			fns.push(fn() => [i, x, y, localFn()[2]])
		}

		var wfns := []
		var k := 0
		while k < 3 {
			v := k * 100
			wfns.push(fn() => v)
			k += 1
		}

		var tasks := []
		for item in ["a", "b", "c"] {
			tag := item
			tasks.push(go (fn() => tag + "!")())
		}

		return [
			[fns[0](), fns[1](), fns[2]()],
			[wfns[0](), wfns[1](), wfns[2]()],
			await.all(tasks),
		]
	}`
	val := run(t, code)
	want := `[[[0, 10, 10, 10], [1, 20, 21, 21], [2, 30, 32, 32]], [0, 100, 200], ["a!", "b!", "c!"]]`
	if val.String() != want {
		t.Fatalf("got  %s\nwant %s", val, want)
	}
}

func TestBareScriptAndInitLoopCaptures(t *testing.T) {
	t.Run("bare_script", func(t *testing.T) {
		val, err := evie.Eval(`
			var fns := []
			for i, x in [10, 20, 30] {
				y := x + i
				fns.push(fn() => [i, x, y])
			}
			return [fns[0](), fns[1](), fns[2]()]
		`)
		if err != nil {
			t.Fatalf("Eval failed: %v", err)
		}
		want := `[[0, 10, 10], [1, 20, 21], [2, 30, 32]]`
		if val.String() != want {
			t.Fatalf("got %s, want %s", val, want)
		}
	})

	t.Run("package_init", func(t *testing.T) {
		val := run(t, `package main
			var fns := []
			for x in [1, 2, 3] {
				fns.push(fn() => x * 10)
			}
			fn main() => [fns[0](), fns[1](), fns[2]()]`)
		if val.String() != "[10, 20, 30]" {
			t.Fatalf("got %s, want [10, 20, 30]", val)
		}
	})

	t.Run("recursive_fn_and_class_in_loop", func(t *testing.T) {
		val := run(t, `package main
			fn main() {
				var fns := []
				var classes := []
				for base in [1, 2, 3] {
					fn sumTo(n) {
						if n <= 0 { return base }
						return n + sumTo(n - 1)
					}
					fns.push(sumTo)

					class Box(v) {
						fn total() => this.v + base
					}
					classes.push(Box(10))
				}
				return [
					[fns[0](2), fns[1](2), fns[2](2)],
					[classes[0].total(), classes[1].total(), classes[2].total()],
				]
			}`)
		want := "[[4, 5, 6], [11, 12, 13]]"
		if val.String() != want {
			t.Fatalf("got %s, want %s", val, want)
		}
	})
}

// break and continue outside a loop or across a function boundary are rejected at compile time
func TestBreakAndContinueValidation(t *testing.T) {
	cases := map[string]string{
		"package main\nfn main() { break }":                                         "break outside of a loop or switch",
		"package main\nfn main() { continue }":                                      "continue outside of a loop",
		"package main\nfn main() { switch 1 { case 1: continue } }":                 "continue outside of a loop",
		"package main\nfn f() { break }\nfn main() { while true { f() } }":          "break outside of a loop or switch",
		"package main\nfn main() { while true { f := fn() { break }\n f() } }":      "break outside of a loop or switch",
		"package main\nfn main() { for x in [1] { f := fn() { continue }\n f() } }": "continue outside of a loop",
	}
	for src, want := range cases {
		_, err := runCode(src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want error containing %q, got %v", src, want, err)
		}
	}
}

// local classes, class body validation, postfix '?', unary '+', and block value consistency
func TestCompilerAndGrammarEdgeCases(t *testing.T) {
	// 1. Local class declaration with inheritance and methods
	localClassCode := `package main
	fn main() {
		class Base(x) {
			fn getX() => this.x
		}
		class Sub(x, y) : Base {
			fn sum() => this.getX() + this.y
		}
		s := Sub(10, 32)
		return [s.sum(), s is Sub, s is Base]
	}`
	if val := run(t, localClassCode); val.String() != "[42, true, true]" {
		t.Fatalf("local class: got %s", val)
	}

	// 2. Existence check on string and buffer indices
	existsCode := `package main
	fn main() {
		return ["hi"[0]?, "hi"[2]?, "hi"[-1]?, "hi"[0.5]?]
	}`
	if val := run(t, existsCode); val.String() != "[true, false, true, false]" {
		t.Fatalf("string index exists: got %s", val)
	}

	// 3. Unary '+' on numbers and runtime error on non-numbers
	if val := run(t, "x := 7\nreturn +x + +(3 + 4)"); val.String() != "14" {
		t.Fatalf("unary plus: got %s", val)
	}
	if _, err := runCode(`return +("hello")`); err == nil || !strings.Contains(err.Error(), "cannot apply unary '+' to a string") {
		t.Fatalf("expected unary '+' type error, got %v", err)
	}

	// 4. Compile errors on invalid '?' target and non-method/non-field in class body
	for src, want := range map[string]string{
		"package main\nfn main() { x := 1\n return x? }":       "'?' can only check a field or index access",
		"package main\nclass Bad() { return 1 }\nfn main() {}": "only methods ('fn') and field declarations are allowed inside a class body",
		"package main\nclass Dup() { fn f() {}\n fn f() {} }":  "method 'f' is declared twice on Dup",
	} {
		_, err := runCode(src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want error containing %q, got %v", src, want, err)
		}
	}

	// 5. await.any failure is located
	_, err := runCode(`package main
	fn fail() { throw error("nope") }
	fn main() {
		return await.any(go fail(), go fail())
	}`)
	if err == nil || !strings.Contains(err.Error(), "all tasks failed in await.any") {
		t.Fatalf("expected await.any failure, got %v", err)
	}

	// 6. Single-statement block in catch behaves identically with fast paths on and off
	for _, noFast := range []bool{false, true} {
		eng := evie.New(evie.Options{Debug: evie.Debug{NoFastPaths: noFast}})
		v, err := eng.Eval("return [catch { 42 }, catch(42)]")
		if err != nil {
			t.Fatalf("NoFastPaths=%v: %v", noFast, err)
		}
		if v.String() != "[nil, 42]" {
			t.Fatalf("NoFastPaths=%v: got %s, want [nil, 42]", noFast, v)
		}
	}
}

// cyclic arrays, objects and class instances format and convert to Go without overflowing the stack
func TestCyclicValuesAndMainArgs(t *testing.T) {
	cyclicCode := `package main
	class Node(val, next) {}
	fn main() {
		a := [1, 2]
		a.push(a)

		o := {k: "v"}
		o.self = o

		n := Node(10, nil)
		n.next = n

		shared := [7]
		dag := [shared, shared]

		return [string(a), string(o), string(n), string(dag), a == a, o == o, n == n]
	}`
	val := run(t, cyclicCode)
	want := `["[1, 2, [...]]", "{k: "v", self: {...}}", "Node{val: 10, next: Node{...}}", "[[7], [7]]", true, true, true]`
	if val.String() != want {
		t.Fatalf("cyclic formatting:\ngot  %s\nwant %s", val.String(), want)
	}

	// Verify ToGo() on cyclic array terminates and preserves structure
	eng := evie.New()
	cyclicArr, err := eng.Eval("a := [1]\na.push(a)\nreturn a")
	if err != nil {
		t.Fatal(err)
	}
	goVal := cyclicArr.ToGo()
	slice, ok := goVal.([]any)
	if !ok || len(slice) != 2 || slice[0] != float64(1) {
		t.Fatalf("unexpected ToGo result: %#v", goVal)
	}

	// Verify Engine Options.Args passes arguments when main accepts them, and ignores them when main() takes 0 args
	dir := t.TempDir()
	variadicMain := filepath.Join(dir, "var_main.ev")
	if err := os.WriteFile(variadicMain, []byte("package main\nfn main(...args) => args.join(\":\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := evie.New(evie.Options{Args: []string{"alpha", "beta", "gamma"}}).RunFile(variadicMain)
	if err != nil || res.String() != "alpha:beta:gamma" {
		t.Fatalf("variadic main with Args: got %v, %v", res, err)
	}

	zeroArgMain := filepath.Join(dir, "zero_main.ev")
	if err := os.WriteFile(zeroArgMain, []byte("package main\nfn main() => 99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = evie.New(evie.Options{Args: []string{"unused1", "unused2"}}).RunFile(zeroArgMain)
	if err != nil || res.String() != "99" {
		t.Fatalf("0-arg main with Args: got %v, %v", res, err)
	}
}

// a blocking native that panics or calls back into evie keeps the single-runner lock consistent
func TestBlockingNativePanicAndCallback(t *testing.T) {
	pkg := vm.NewPackage("host")
	pkg.Func("boom", 0, func(c vm.Call) (vm.Value, error) {
		panic("boom from blocking native")
	}).Blocking = true

	var active atomic.Int32
	var overlap atomic.Int32
	enter := func() {
		if active.Add(1) > 1 {
			overlap.Add(1)
		}
	}
	leave := func() {
		active.Add(-1)
	}
	pkg.Func("step", 0, func(c vm.Call) (vm.Value, error) {
		enter()
		defer leave()
		return vm.Value{}, nil
	})
	pkg.Func("withCallback", 1, func(c vm.Call) (vm.Value, error) {
		fn, err := c.Callable(0)
		if err != nil {
			return vm.Value{}, err
		}
		return c.Call(fn, vm.BoxNumber(21))
	}).Blocking = true

	eng := evie.New(evie.Options{Packages: []*vm.Package{pkg}})
	if _, err := eng.Eval(`package main imports("host")
	fn callBoom() {
		return host.boom()
	}
	fn main() {
		// 1. Panic in blocking native running inside a background task
		t := go host.boom()
		c := catch(await t)
		if c == nil {
			throw error("expected background panic to be caught as an error")
		}

		// 2. Callback from blocking native while another fiber runs
		worker := go (fn() {
			var i := 0
			while i < 200 {
				host.step()
				i += 1
			}
		})()
		res := host.withCallback(fn(x) {
			host.step()
			return x * 2
		})
		await worker
		if res != 42 {
			throw error("unexpected callback result")
		}
	}`); err != nil {
		t.Fatal(err)
	}
	// A direct panic in a blocking native recovers cleanly and leaves the engine lock intact
	if _, err := eng.Call("main.callBoom"); err == nil || !strings.Contains(err.Error(), "boom from blocking native") {
		t.Fatalf("expected recovered panic error, got %v", err)
	}
	if _, err := eng.Call("main.main"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := overlap.Load(); n != 0 {
		t.Fatalf("blocking callback overlapped with worker %d times", n)
	}
}

// blocking natives wrapped with Func* or Reflect marshal mutable arguments under the runner lock
func TestBlockingNativeArgMarshaling(t *testing.T) {
	pkg := vm.NewPackage("host")
	pkg.Native("inspectAny", vm.Func1(func(v any) int {
		m, ok := v.(map[string]any)
		if !ok {
			return 0
		}
		return len(m)
	})).Blocking = true
	pkg.Native("inspectMap", vm.Reflect(func(m map[string]int) int {
		return len(m)
	})).Blocking = true

	eng := evie.New(evie.Options{Packages: []*vm.Package{pkg}})
	if _, err := eng.Eval(`package main imports("host")
	fn main() {
		shared := {a: 1, b: 2}
		writer := go (fn() {
			var i := 0
			while i < 200 {
				shared["k" + string(i)] = i
				i += 1
			}
		})()
		var ok := true
		var j := 0
		while j < 50 {
			if host.inspectAny(shared) < 2 || host.inspectMap(shared) < 2 {
				ok = false
			}
			j += 1
		}
		await writer
		return ok
	}`); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Call("main.main")
	if err != nil || res.String() != "true" {
		t.Fatalf("got %v, %v", res, err)
	}
}

// [:] slice syntax and .slice(start[, end]) work across arrays, strings and buffers
func TestSlicingSyntaxAndMethods(t *testing.T) {
	eng := evie.New()
	eng.SetStatic("sampleBuf", vm.BoxBuffer([]byte("abcdef")))
	_, err := eng.Eval(`package main
	fn main() {
		a := [10, 20, 30, 40, 50]
		copyA := a[:]
		copyA[0] = 99
		arrSlices := [a[1:4], a[:2], a[3:], a[1:-1], a.slice(2), a.slice(1, 3), a[0]]

		s := "hello world"
		strSlices := [s[0:5], s[:5], s[6:], s[1:-1], s[:], s.slice(6), s.slice(0, 5)]

		b := sampleBuf
		bufSlices := [string(b[1:4]), string(b[:3]), string(b[3:]), string(b[1:-1]), string(b[:]), string(b.slice(2)), string(b.slice(1, 4))]

		errs := [
			catch(a["bad":2]).message,
			catch(s[0:1.5]).message,
			catch((123)[0:1]).message,
		]
		return [arrSlices, strSlices, bufSlices, errs]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := eng.Call("main.main")
	if err != nil {
		t.Fatal(err)
	}
	want := `[[[20, 30, 40], [10, 20], [40, 50], [20, 30, 40], [30, 40, 50], [20, 30], 10], ["hello", "hello", "world", "ello worl", "hello world", "world", "hello"], ["bcd", "abc", "def", "bcde", "abcdef", "cdef", "bcd"], ["slice start index must be a number, got 'string'", "slice end index must be an integer, got 1.5", "cannot slice value of type 'number'"]]`
	if res.String() != want {
		t.Fatalf("got  %s\nwant %s", res, want)
	}
}

// buffers support construction from strings/arrays/sizes, subscript mutation, compound assignment and push
func TestBufferMutationAndConstruction(t *testing.T) {
	val := run(t, `package main
	fn main() {
		b1 := buffer("abc")
		b1[0] = 65
		b1[-1] += 1
		b1.push(33).push("?")

		b2 := buffer([72, 105])
		b3 := buffer(3)
		b3[1] = 42

		errs := [
			catch { b1[0] = 300 }.message,
			catch { b1[0] = -1 }.message,
			catch { b1[10] = 65 }.message,
			catch(buffer(-2)).message,
			catch(buffer(1000000000000)).message,
		]
		return [b1.string(), b2.string(), [b3[0], b3[1], b3[2]], errs]
	}`)
	want := `["Abd!?", "Hi", [0, 42, 0], ["buffer element must be an integer in 0..255, got 300", "buffer element must be an integer in 0..255, got -1", "buffer index 10 out of bounds (length 5)", "buffer: size must not be negative, got -2", "buffer: size 1000000000000 exceeds the maximum buffer size (67108864 bytes)"]]`
	if val.String() != want {
		t.Fatalf("got  %s\nwant %s", val, want)
	}
}

// super invokes overridden methods up a multi-level inheritance chain and inside closures
func TestClassSuperCalls(t *testing.T) {
	val := run(t, `package main
	class Animal(name) {
		fn speak() => this.name + " makes a sound"
	}
	class Dog(name, breed) : Animal {
		fn speak() => super.speak() + " (woof from " + this.breed + ")"
	}
	class GuideDog(name, breed, owner) : Dog {
		fn speak() => super.speak() + " for " + this.owner
		fn deferredSpeak() {
			fn inner() => super.speak()
			bound := super.speak
			return [inner(), bound()]
		}
	}
	fn main() {
		g := GuideDog("Rex", "Lab", "Alice")
		return [g.speak(), g.deferredSpeak()]
	}`)
	want := `["Rex makes a sound (woof from Lab) for Alice", ["Rex makes a sound (woof from Lab)", "Rex makes a sound (woof from Lab)"]]`
	if val.String() != want {
		t.Fatalf("got  %s\nwant %s", val, want)
	}

	for src, wantErr := range map[string]string{
		"package main\nfn main() => super.foo()":                               "'super' can only be used inside a class method",
		"package main\nclass A() { fn f() => super.f() }\nfn main() {}":        "class 'A' has no parent class",
		"package main\nfn main() { x := super }":                               "expected '.' and a method name after 'super'",
		"package main\nclass A() {}\nclass B() : A { fn f() { super.g = 1 } }": "cannot assign to 'super.g'",
	} {
		if _, err := runCode(src); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("%q: want error containing %q, got %v", src, wantErr, err)
		}
	}
}

// fixed-shape classes support mutable (var) and constant (:=) body fields,
// per-instance dynamic initializers, ancestor-first inheritance, and reject
// undeclared or constant field mutations
func TestFixedShapeClassesAndBodyFields(t *testing.T) {
	// 1. Mutable & constant body fields, dynamic per-instance initializers, compound assignment, subscript access, ?
	val := run(t, `package main
	var seq := 0
	fn nextId() {
		seq += 1
		return seq
	}
	class User(first, last) {
		role := "member"
		full := this.first + " " + this.last
		id := nextId()
		var email := ""
		var score := 0
		var tags := []

		fn addScore(n) {
			this.score += n
		}
		fn tag(t) {
			this.tags.push(t)
		}
	}
	fn main() {
		a := User("Ada", "Lovelace")
		b := User("Alan", "Turing")
		a.email = "ada@example.com"
		a.addScore(10)
		a.score += 5
		a["score"] += 2
		a.tag("math")
		b.tag("cs")
		b["email"] = "alan@example.com"
		return [
			a.role, a.full, a.id, a.email, a.score, a.tags,
			b.role, b.full, b.id, b.email, b.score, b.tags,
			a.email?, a.missing?, a["role"]?, a["nope"]?,
		]
	}`)
	want := `["member", "Ada Lovelace", 1, "ada@example.com", 17, ["math"], "member", "Alan Turing", 2, "alan@example.com", 0, ["cs"], true, false, true, false]`
	if val.String() != want {
		t.Fatalf("body fields:\ngot  %s\nwant %s", val, want)
	}

	// 2. Inheritance: parent declared after child, multi-level, overrides (constant & constructor param), and local runtime parent
	val = run(t, `package main
	class Child(name, tag) : Base {
		kind := "child"
		var extra := this.name + ":" + this.tag
	}
	class GrandChild(name) : Child {
		tag := "fixed"
		var items := [this.name]
	}
	class Base(name) {
		kind := "base"
		var tag := "dyn-" + this.name
		var baseList := []
		fn describe() => this.kind + "/" + this.tag + "/" + string(this.baseList.len())
	}
	fn makeLocal(ParentClass, prefix) {
		class LocalSub(name) : ParentClass {
			tag := "override"
			var note := prefix + ":" + this.name
		}
		return LocalSub("Zed")
	}
	fn main() {
		c := Child("A", "custom")
		c.baseList.push(1)
		g := GrandChild("B")
		l := makeLocal(Base, "local")
		l.baseList.push(9)
		return [
			c.describe(), c.extra,
			g.describe(), g.kind, g.tag, g.items, g.baseList.len(),
			l.describe(), l.note,
		]
	}`)
	want = `["child/custom/1", "A:custom", "child/fixed/0", "child", "fixed", ["B"], 0, "base/override/1", "local:Zed"]`
	if val.String() != want {
		t.Fatalf("inheritance body fields:\ngot  %s\nwant %s", val, want)
	}

	// 3. Polymorphic 2-entry & megamorphic member call sites (methods + callable fields)
	val = run(t, `package main
	class A(v) { fn f(x) => this.v + x }
	class B(v) { fn f(x) => this.v * x }
	class C(f) {}
	fn main() {
		pair := [A(10), B(10), A(20), B(20)]
		var sum2 := 0
		for item in pair {
			sum2 += item.f(3)
		}
		trio := [A(1), B(2), C(fn(x) => x + 100), A(3), B(4)]
		var sum3 := 0
		for item in trio {
			sum3 += item.f(5)
		}
		return [sum2, sum3]
	}`)
	if val.String() != "[126, 149]" {
		t.Fatalf("polymorphic memberSite: got %s, want [126, 149]", val)
	}

	// 4. Runtime errors: undeclared field (with did-you-mean) and constant field mutation
	for src, wants := range map[string][]string{
		`package main
		class Person(name) { var email := "" }
		fn main() { p := Person("A"); p.emial = "x" }`: {"class 'Person' has no field 'emial'", "did you mean 'email'?"},
		`package main
		class Person(name) {}
		fn main() { p := Person("A"); p["extra"] = 1 }`: {"class 'Person' has no field 'extra'"},
		`package main
		class Person(name) {}
		fn main() { p := Person("A"); p.extra += 1 }`: {"class 'Person' has no field 'extra'"},
		`package main
		class Person(name) { role := "user" }
		fn main() { p := Person("A"); p.role = "admin" }`: {"cannot assign to constant field 'role' on class 'Person'"},
		`package main
		class Person(name) { role := "user" }
		fn main() { p := Person("A"); p.role += "!" }`: {"cannot assign to constant field 'role' on class 'Person'"},
		`package main
		class Person(name) { role := "user" }
		fn main() { p := Person("A"); p["role"] = "admin" }`: {"cannot assign to constant field 'role' on class 'Person'"},
		`package main
		class Person(name) { role := "user" }
		fn main() { p := Person("A"); p["role"] += "!" }`: {"cannot assign to constant field 'role' on class 'Person'"},
		`package main
		class Person(name) {
			role := "user"
			fn promote() { this.role = "admin" }
		}
		fn main() { Person("A").promote() }`: {"cannot assign to constant field 'role' on class 'Person'"},
		`package main
		class Person(name) {
			role := "user"
			fn promote() { this.role += "!" }
		}
		fn main() { Person("A").promote() }`: {"cannot assign to constant field 'role' on class 'Person'"},
	} {
		code := strings.ReplaceAll(src, "; ", "\n")
		_, err := runCode(code)
		if err == nil {
			t.Errorf("%q: expected error, got nil", src)
			continue
		}
		formatted := evie.Format(err, false)
		for _, w := range wants {
			if !strings.Contains(formatted, w) {
				t.Errorf("%q: want error containing %q, got:\n%s", src, w, formatted)
			}
		}
	}

	// 5. Compile errors: duplicate fields, field/param clash, field/method clash
	for src, wantErr := range map[string]string{
		"package main\nclass A() { var x := 1\n var x := 2 }\nfn main() {}": "field 'x' is declared twice on A",
		"package main\nclass A(x) { var x := 1 }\nfn main() {}":             "field 'x' is declared twice on A",
		"package main\nclass A(x) { fn x() {} }\nfn main() {}":              "cannot declare method 'x' on A: a field with that name already exists",
		"package main\nclass A() { var x := 1\n fn x() {} }\nfn main() {}":  "cannot declare method 'x' on A: a field with that name already exists",
	} {
		if _, err := runCode(src); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("%q: want error containing %q, got %v", src, wantErr, err)
		}
	}

	// 6. ToGo() and MarshalJSON() reflect declared constructor + body fields
	v := run(t, `package main
	class Item(name) {
		kind := "book"
		var qty := 3
	}
	fn main() => Item("Go")`)
	raw, err := v.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"name":"Go","kind":"book","qty":3}` {
		t.Fatalf("MarshalJSON = %s", raw)
	}
	if m, ok := v.ToGo().(map[string]any); !ok || m["__class"] != "Item" || m["name"] != "Go" || m["kind"] != "book" || m["qty"] != float64(3) || len(m) != 4 {
		t.Fatalf("ToGo = %#v", v.ToGo())
	}
}
