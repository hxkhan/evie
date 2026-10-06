package vm

import (
	"context"
	"testing"

	"github.com/hxkhan/evie/diag"
)

// runs bound to a context forget it once they and their tasks are done, so a
// long-lived context does not collect children
func TestScopedStoppersAreReleased(t *testing.T) {
	evm := New(Options{})
	_, err := evm.EvalSource(diag.NewSource("", []byte(`package main
	fn work(n) => n * 2
	fn spawner() {
		t := go work(4)
		return await t
	}`)))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for range 50 {
		if _, err := evm.CallContext(ctx, "main.work", 1); err != nil {
			t.Fatal(err)
		}
		if _, err := evm.CallContext(ctx, "main.spawner"); err != nil {
			t.Fatal(err)
		}
	}
	evm.WaitForNoActivity()

	evm.rt.stops.mu.Lock()
	left := len(evm.rt.stops.scoped)
	evm.rt.stops.mu.Unlock()
	if left != 0 {
		t.Fatalf("%d context-bound stoppers were never released", left)
	}
}
