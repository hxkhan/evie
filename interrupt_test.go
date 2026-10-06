package evie_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

const runaway = `package main imports("time")

fn spin() {
	var i := 0
	while true {
		i += 1
	}
}

fn fib(n) {
	if n < 2 {
		return n
	}
	return fib(n - 1) + fib(n - 2)
}

fn walk() {
	var total := 0
	for x in [1, 2, 3, 4, 5, 6, 7, 8] {
		for y in [1, 2, 3, 4, 5, 6, 7, 8] {
			total += fib(x + y + 20)
		}
	}
	return total
}

fn swallow() {
	while true {
		catch(spin())
	}
}

fn sleepy() {
	catch(time.sleep(60000))
	return "woke"
}

fn waiter() {
	t := go time.sleep(60000)
	return await t
}

fn background() {
	go spin()
	go spin()
	return 1
}

fn add(a, b) => a + b
`

// load gives an engine with the runaway program loaded
func load(t *testing.T) *evie.Engine {
	t.Helper()
	eng := evie.New()
	if _, err := eng.Load("runaway.ev", []byte(runaway)); err != nil {
		t.Fatal(err)
	}
	return eng
}

// within fails the test when fn has not returned after d
func within(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		fn()
	}()
	select {
	case <-finished:
	case <-time.After(d):
		t.Fatalf("%s did not stop within %v", what, d)
	}
}

// Interrupt stops loops, recursion and code that tries to catch the stop
func TestInterruptStopsRunawayCode(t *testing.T) {
	for _, fn := range []string{"spin", "fib", "swallow", "sleepy", "waiter"} {
		t.Run(fn, func(t *testing.T) {
			eng := load(t)
			timer := time.AfterFunc(30*time.Millisecond, eng.Interrupt)
			defer timer.Stop()

			var err error
			within(t, 5*time.Second, fn, func() {
				var args []any
				if fn == "fib" {
					args = []any{80}
				}
				_, err = eng.Call("main."+fn, args...)
			})
			if !errors.Is(err, vm.ErrInterrupted) {
				t.Fatalf("expected an interrupted error, got %v", err)
			}
		})
	}
}

// the error says where the script was when it stopped
func TestInterruptedErrorHasLocation(t *testing.T) {
	eng := load(t)
	time.AfterFunc(30*time.Millisecond, eng.Interrupt)
	_, err := eng.Call("main.spin")
	out := evie.Format(err, false)
	if !strings.Contains(out, "interrupted") || !strings.Contains(out, "runaway.ev:5") {
		t.Fatalf("unexpected report:\n%s", out)
	}
}

// a deadline stops the call with the context's cause, wherever it waits
func TestCallContextDeadline(t *testing.T) {
	for _, fn := range []string{"spin", "walk", "sleepy", "waiter"} {
		t.Run(fn, func(t *testing.T) {
			eng := load(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()

			var err error
			within(t, 5*time.Second, fn, func() {
				_, err = eng.CallContext(ctx, "main."+fn)
			})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected a deadline error, got %v", err)
			}
		})
	}
}

// a context that is already done stops the call before it does anything
func TestCallContextAlreadyCancelled(t *testing.T) {
	eng := load(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := eng.CallContext(ctx, "main.spin"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected a cancelled error, got %v", err)
	}
}

// a call that finishes in time is unaffected, and its context is released
func TestCallContextFinishes(t *testing.T) {
	eng := load(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for range 100 {
		v, err := eng.CallContext(ctx, "main.add", 2, 3)
		if err != nil || intOf(v) != 5 {
			t.Fatalf("add(2, 3) = %v, %v", v, err)
		}
	}
}

// cancelling one call leaves the other calls of the instance alone
func TestCallContextStopsOnlyItsCall(t *testing.T) {
	eng := evie.New()
	_, err := eng.Load("two.ev", []byte(`package main imports("time")
	fn spin() {
		while true {
			time.sleep(1)
		}
	}
	fn steady() {
		var total := 0
		var i := 0
		while i < 50 {
			time.sleep(2)
			total += i
			i += 1
		}
		return total
	}`))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	spun := make(chan error, 1)
	go func() {
		_, err := eng.CallContext(ctx, "main.spin")
		spun <- err
	}()

	var v vm.Value
	within(t, 10*time.Second, "steady", func() { v, err = eng.Call("main.steady") })
	if err != nil || intOf(v) != 1225 {
		t.Fatalf("steady() = %v, %v", v, err)
	}
	if err := <-spun; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected spin to hit its deadline, got %v", err)
	}
}

// tasks a call leaves running stop with an interrupt, and the instance stays usable
func TestInterruptStopsBackgroundTasks(t *testing.T) {
	eng := load(t)
	if _, err := eng.Call("main.background"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	eng.Interrupt()
	within(t, 5*time.Second, "background tasks", eng.WaitForNoActivity)

	v, err := eng.Call("main.add", 1, 2)
	if err != nil || intOf(v) != 3 {
		t.Fatalf("add(1, 2) after an interrupt = %v, %v", v, err)
	}
}

// a program stops with its context while it loads and while main runs
func TestRunFileContext(t *testing.T) {
	cases := map[string]string{
		"init": `package main
		fn forever() {
			while true {}
		}
		var stuck := forever()
		fn main() => 1`,
		"main": `package main
		fn main() {
			while true {}
		}`,
		"import": `package main imports("./lib")
		fn main() => lib.value`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "main.ev")
			if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			lib := "package lib\nfn forever() {\n\twhile true {}\n}\nvar value := forever()\n"
			if err := os.WriteFile(filepath.Join(dir, "lib.ev"), []byte(lib), 0o644); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			var err error
			within(t, 5*time.Second, name, func() {
				_, err = evie.New().RunFileContext(ctx, file)
			})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected a deadline error, got %v", err)
			}
		})
	}
}

// a bare script stops with its context too
func TestEvalContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	var err error
	within(t, 5*time.Second, "eval", func() {
		_, err = evie.New().EvalContext(ctx, "var i := 0\nwhile true { i += 1 }")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a deadline error, got %v", err)
	}
}
