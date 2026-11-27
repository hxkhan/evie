package evie_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

// callDepthEngine is an engine with a native that calls back into the script,
// as hosts do with callbacks, and a native that panics
func callDepthEngine(t *testing.T, opts evie.Options, src string) *evie.Engine {
	t.Helper()
	host := vm.NewPackage("host")
	host.Native("callback", vm.NewFunc("callback", 1, func(c vm.Call) (vm.Value, error) {
		return c.Call(c.Arg(0))
	}))
	host.Native("boom", vm.NewFunc("boom", 0, func(c vm.Call) (vm.Value, error) {
		panic("host bug")
	}))
	opts.Packages = append(opts.Packages, host)
	eng := evie.New(opts)
	if _, err := eng.Eval(src); err != nil {
		t.Fatal(err)
	}
	return eng
}

// wantStackOverflow fails unless err is the call depth error
func wantStackOverflow(t *testing.T, name string, err error) {
	t.Helper()
	var e *vm.Error
	if !errors.As(err, &e) {
		t.Fatalf("%s: expected a stack overflow error, got %v", name, err)
	}
	if e.Kind() != vm.KindStackOverflow {
		t.Fatalf("%s: expected kind %q, got %q (%v)", name, vm.KindStackOverflow, e.Kind(), err)
	}
}

// recursion that never ends is an error the host gets back, whatever path the
// calls take, instead of a Go stack overflow that kills the process
func TestRunawayRecursionIsAnError(t *testing.T) {
	eng := callDepthEngine(t, evie.Options{}, `package main imports("host")
	fn direct(n) { return 1 + direct(n + 1) }
	fn short(n) => 1 + short(n + 1)
	fn noLocals() => noLocals()
	fn ping(n) => pong(n)
	fn pong(n) => ping(n)
	fn variadic(...xs) => variadic(1, 2)
	fn throughMap(n) => [1].map(fn(x) => throughMap(n))
	fn throughHost(n) => host.callback(fn() => throughHost(n))
	fn closure() {
		var f := nil
		f = fn(n) => f(n + 1)
		return f(0)
	}
	class Node(n) {
		fn down() => this.down()
	}
	fn method() => Node(1).down()
	fn bound() {
		m := Node(1).down
		return m()
	}
	class Loop() {
		var next := Loop()
	}
	fn constructor() => Loop()
	fn inTask() => await go direct(0)
	`)
	for _, name := range []string{"direct", "short", "noLocals", "ping", "variadic",
		"throughMap", "throughHost", "closure", "method", "bound", "constructor", "inTask"} {
		args := []any{}
		if name == "direct" || name == "short" || name == "ping" || name == "throughMap" || name == "throughHost" {
			args = append(args, 0)
		}
		_, err := eng.Call("main."+name, args...)
		wantStackOverflow(t, name, err)
	}
}

// a script can catch the error and carry on, and the next run starts from zero
func TestStackOverflowIsCatchable(t *testing.T) {
	eng := callDepthEngine(t, evie.Options{}, `package main
	fn forever(n) => 1 + forever(n + 1)
	fn depth(n) {
		if n == 0 { return 0 }
		return 1 + depth(n - 1)
	}
	fn main() {
		e := catch(forever(0))
		return [e.kind, depth(9000)]
	}
	`)
	for range 3 {
		res, err := eng.Call("main.main")
		if err != nil {
			t.Fatal(err)
		}
		if got := res.String(); got != `["stackOverflow", 9000]` {
			t.Fatalf("got %s", got)
		}
	}
}

// Options.MaxCallDepth sets the limit, and zero means the default
func TestMaxCallDepthOption(t *testing.T) {
	const src = `package main
	fn depth(n) {
		if n == 0 { return 0 }
		return 1 + depth(n - 1)
	}
	fn run(n) => depth(n)
	`
	cases := []struct {
		limit, depth int
		ok           bool
	}{
		{0, 9_000, true},
		{0, 10_000, false},
		{100, 97, true}, // run, then depth for n down to 0: n + 2 calls
		{100, 98, true},
		{100, 99, false},
		{100_000, 90_000, true},
		{100_000, 100_000, false},
	}
	for _, c := range cases {
		eng := callDepthEngine(t, evie.Options{MaxCallDepth: c.limit}, src)
		res, err := eng.Call("main.run", c.depth)
		if c.ok {
			if err != nil {
				t.Fatalf("limit %d, depth %d: %v", c.limit, c.depth, err)
			}
			if n, _ := res.AsFloat64(); int(n) != c.depth {
				t.Fatalf("limit %d, depth %d: got %v", c.limit, c.depth, res)
			}
		} else {
			wantStackOverflow(t, fmt.Sprintf("limit %d, depth %d", c.limit, c.depth), err)
		}
	}
}

// a host panic deep in a callback leaves the depth as it was, so the script
// that catches it can still call as deep as before. A bound method passed to a
// native is called under its own panic guard on the same fiber.
func TestPanicRestoresCallDepth(t *testing.T) {
	eng := callDepthEngine(t, evie.Options{MaxCallDepth: 100}, `package main imports("host")
	fn depth(n) {
		if n == 0 { return 0 }
		return 1 + depth(n - 1)
	}
	class Bomb() {
		fn fall(n) {
			if n == 0 { return host.boom() }
			return this.fall(n - 1)
		}
		fn start() => this.fall(60)
	}
	fn main() {
		e := catch(host.callback(Bomb().start))
		return [e.message, depth(95)]
	}
	`)
	res, err := eng.Call("main.main")
	if err != nil {
		t.Fatal(err)
	}
	if got := res.String(); !strings.Contains(got, "host bug") || !strings.HasSuffix(got, ", 95]") {
		t.Fatalf("got %s", got)
	}
}

// the trace of a runaway recursion is short enough to read
func TestStackOverflowTraceIsShort(t *testing.T) {
	eng := callDepthEngine(t, evie.Options{}, `package main
	fn forever(n) => 1 + forever(n + 1)
	fn ping(n) => pong(n + 1)
	fn pong(n) => ping(n + 1)
	`)
	_, err := eng.Call("main.forever", 0)
	text := evie.Format(err, false)
	if !strings.Contains(text, "(10000 times)") || strings.Count(text, "\n") > 12 {
		t.Fatalf("expected the repeated frame once with a count, got:\n%s", text)
	}

	_, err = eng.Call("main.ping", 0)
	text = evie.Format(err, false)
	if !strings.Contains(text, "more ...") || strings.Count(text, "\n") > 32 {
		t.Fatalf("expected a long trace cut short, got:\n%s", text)
	}
	var e *vm.Error
	if errors.As(err, &e); len(e.Trace) < 9_000 {
		t.Fatalf("the error should keep its whole trace, has %d frames", len(e.Trace))
	}
}
