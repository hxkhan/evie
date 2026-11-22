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

fn waitAny() {
	t1 := go time.sleep(60000)
	t2 := go time.sleep(60000)
	return await.any(t1, t2)
}

fn waitRace() {
	t1 := go time.sleep(60000)
	t2 := go time.sleep(60000)
	return await.race(t1, t2)
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
	for _, fn := range []string{"spin", "fib", "swallow", "sleepy", "waiter", "waitAny", "waitRace"} {
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
	for _, fn := range []string{"spin", "walk", "sleepy", "waiter", "waitAny", "waitRace"} {
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
	within(t, 5*time.Second, "background tasks", eng.Wait)

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

// a package that evaluated source imports by path, outside any bundle, runs
// its initializers under the same context
func TestEvalContextStopsPathImports(t *testing.T) {
	dir := t.TempDir()
	lib := "package lib\nfn forever() {\n\twhile true {}\n}\nvar value := forever()\n"
	if err := os.WriteFile(filepath.Join(dir, "lib.ev"), []byte(lib), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	var err error
	within(t, 5*time.Second, "eval", func() {
		_, err = evie.New(evie.Options{Root: dir}).EvalContext(ctx, `package main imports("./lib")
fn main() => lib.value`)
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a deadline error, got %v", err)
	}
}

// io.readln and io.prompt stop when the engine is interrupted or the call context expires
func TestInterruptAndContextStopReadln(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = origStdin
		_ = w.Close()
		_ = r.Close()
	}()

	eng := evie.New()
	if _, err := eng.Eval(`package main imports("io")
	fn ask() => io.readln()`); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	within(t, 5*time.Second, "readln deadline", func() {
		_, err = eng.CallContext(ctx, "main.ask")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a deadline error, got %v", err)
	}

	timer := time.AfterFunc(30*time.Millisecond, eng.Interrupt)
	defer timer.Stop()
	within(t, 5*time.Second, "readln interrupt", func() {
		_, err = eng.Call("main.ask")
	})
	if !errors.Is(err, vm.ErrInterrupted) {
		t.Fatalf("expected an interrupted error, got %v", err)
	}
}

// a task stopped in a timed-out call produces a catchable error when awaited via await or await.race in a later call
func TestAwaitRaceForeignStopIsCatchable(t *testing.T) {
	eng := evie.New()
	_, err := eng.Load("foreign.ev", []byte(`package main imports("time")
	var saved := nil
	fn start() {
		saved = go time.sleep(60000)
		time.sleep(60000)
	}
	fn inspect() {
		e1 := catch(await saved)
		e2 := catch(await.race(saved))
		return [e1 != nil, e2 != nil]
	}`))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = eng.CallContext(ctx, "main.start")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	eng.Wait()

	res, err := eng.Call("main.inspect")
	if err != nil || res.String() != "[true, true]" {
		t.Fatalf("inspect() = %v, %v", res, err)
	}
}

// task.cancel stops only that task (even if it tries to catch the stop or is
// sleeping/awaiting) and leaves sibling tasks and the caller running
func TestTaskCancel(t *testing.T) {
	eng := evie.New()
	_, err := eng.Eval(`package main imports("time")
	fn spinForever() {
		var i := 0
		while true {
			catch {
				while true { i += 1 }
			}
		}
	}
	fn loopSleep() {
		while true {
			catch {
				time.sleep(2)
			}
		}
	}
	fn startSpinTask() => go spinForever()
	fn main() {
		// 1. Cancel a task before it runs, a looping task with catch, and a sleeping task while a sibling finishes
		tPre := go spinForever()
		tPre.cancel()

		tLoop := go loopSleep()
		tSleep := go time.sleep(60000)
		tGood := go (fn() {
			time.sleep(10)
			return "ok"
		})()

		time.sleep(5)
		tLoop.cancel()
		tSleep.cancel()

		ePre := catch(await tPre)
		eLoop := catch(await tLoop)
		eSleep := catch(await tSleep)
		good := await tGood

		// 2. Cancel a task that is awaiting another task without stopping the inner task
		inner := go (fn() {
			time.sleep(20)
			return 99
		})()
		outer := go (fn() => await inner)()
		time.sleep(5)
		outer.cancel()
		eOuter := catch(await outer)
		innerVal := await inner

		// 3. Cancelling an already completed task preserves its result
		inner.cancel()

		return [ePre.message, eLoop.message, eSleep.message, good, eOuter.message, innerVal, await inner]
	}`)
	if err != nil {
		t.Fatal(err)
	}

	var res vm.Value
	within(t, 5*time.Second, "task.cancel", func() {
		res, err = eng.Call("main.main")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `["cancelled", "cancelled", "cancelled", "ok", "cancelled", 99, 99]`
	if res.String() != want {
		t.Fatalf("got  %s\nwant %s", res, want)
	}

	// 4. Host (*vm.Task).Cancel stops a CPU-spinning task from another goroutine
	taskVal, err := eng.Call("main.startSpinTask")
	if err != nil {
		t.Fatal(err)
	}
	spinTask, _ := taskVal.AsTask()
	time.AfterFunc(20*time.Millisecond, spinTask.Cancel)
	within(t, 5*time.Second, "host Task.Cancel", func() {
		<-spinTask.Done()
	})
	if _, err := spinTask.Result(); !errors.Is(err, vm.ErrCanceled) {
		t.Fatalf("expected ErrCanceled, got %v", err)
	}
}

// await.timeout returns a fast task's result, cancels and times out a slow task,
// and wakes immediately when the engine is interrupted
func TestAwaitTimeout(t *testing.T) {
	eng := evie.New()
	_, err := eng.Eval(`package main imports("time")
	fn main() {
		fast := go (fn() {
			time.sleep(5)
			return "done"
		})()
		rFast := await.timeout(fast, 1000)

		slow := go time.sleep(60000)
		eSlow := catch(await.timeout(slow, 20))
		eAfter := catch(await slow)

		plain := await.timeout(42, 10)
		eType := catch(await.timeout(fast, "bad"))

		return [rFast, eSlow.message, eAfter.message, plain, eType.message]
	}
	fn waitLong() {
		t := go time.sleep(60000)
		return await.timeout(t, 60000)
	}`)
	if err != nil {
		t.Fatal(err)
	}

	var res vm.Value
	within(t, 5*time.Second, "await.timeout", func() {
		res, err = eng.Call("main.main")
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `["done", "await.timeout: timed out after 20ms", "cancelled", 42, "await.timeout: duration must be a number, got string"]`
	if res.String() != want {
		t.Fatalf("got  %s\nwant %s", res, want)
	}

	// Interrupt wakes await.timeout immediately
	timer := time.AfterFunc(30*time.Millisecond, eng.Interrupt)
	defer timer.Stop()
	within(t, 5*time.Second, "interrupt await.timeout", func() {
		_, err = eng.Call("main.waitLong")
	})
	if !errors.Is(err, vm.ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got %v", err)
	}

	// Parse error on wrong argument count
	if _, err := eng.Eval("package p\nfn f() => await.timeout(1)"); err == nil || !strings.Contains(err.Error(), "await.timeout takes 2 arguments") {
		t.Fatalf("expected arity syntax error, got %v", err)
	}
}
