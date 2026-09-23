package vm

import "testing"

// every call path must give back the stack entries and pooled boxes it took,
// including when an argument throws before the callee runs
func TestCallsReleaseFiberStateOnException(t *testing.T) {
	evm := New(Options{})
	_, err := evm.EvalScript([]byte(`
	package main

	struct Box(n) {
		fn add(k) {
			this.n = this.n + k
		}
	}

	fn boom() {
		throw error("boom")
	}

	fn fixed(a, b) => a + b
	fn variadic(a, ...rest) => a
	fn recurse(n, ...rest) {
		if n == 0 {
			return boom()
		}
		return recurse(n - 1, n, boom())
	}

	fn main() {
		dynamicFixed := fixed
		dynamicVariadic := variadic
		b := Box(0)
		var arr := []
		var caught := 0
		var i := 0
		while i < 100 {
			if catch(fixed(1, boom())) is error { caught += 1 }
			if catch(variadic(1, 2, boom())) is error { caught += 1 }
			if catch(variadic(boom())) is error { caught += 1 }
			if catch(recurse(3)) is error { caught += 1 }
			if catch(dynamicFixed(1, boom())) is error { caught += 1 }
			if catch(dynamicVariadic(1, boom())) is error { caught += 1 }
			if catch(b.add(boom())) is error { caught += 1 }
			if catch(arr.push(boom())) is error { caught += 1 }
			if catch(string(boom())) is error { caught += 1 }
			i += 1
		}
		return caught
	}
	`))
	if err != nil {
		t.Fatal(err)
	}

	mainFn, ok := evm.Lookup("main.main")
	if !ok {
		t.Fatal("main.main not found")
	}

	fbr := evm.NewFiber()
	stackBefore, boxesBefore := len(fbr.stack), len(fbr.boxes)

	result, exc := mainFn.Call(fbr)
	if exc != nil {
		t.Fatal(exc)
	}
	if n, _ := result.AsFloat64(); n != 900 {
		t.Fatalf("expected 900 caught errors, got %v", result)
	}

	if len(fbr.stack) != stackBefore {
		t.Errorf("fiber stack leaked: %d entries before, %d after", stackBefore, len(fbr.stack))
	}
	if len(fbr.boxes) != boxesBefore || fbr.debt != 0 {
		t.Errorf("box pool unbalanced: %d free before, %d after, debt %d", boxesBefore, len(fbr.boxes), fbr.debt)
	}
}
