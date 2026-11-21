package evie_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hxkhan/evie"
)

func TestSessionStateAndImmutability(t *testing.T) {
	eng := evie.New()
	defer eng.Close()
	s := eng.NewSession()

	// Immutable binding persists across turns
	if _, hasVal, err := s.Eval("x := 10"); err != nil || hasVal {
		t.Fatalf("decl x: hasVal=%v err=%v", hasVal, err)
	}
	val, hasVal, err := s.Eval("x * 3")
	if err != nil || !hasVal || val.String() != "30" {
		t.Fatalf("x * 3: val=%v hasVal=%v err=%v", val, hasVal, err)
	}

	// Last non-nil result is saved in _
	val, hasVal, err = s.Eval("_ + 12")
	if err != nil || !hasVal || val.String() != "42" {
		t.Fatalf("_ + 12: val=%v hasVal=%v err=%v", val, hasVal, err)
	}

	// Reassigning an immutable binding is still rejected
	if _, _, err := s.Eval("x = 20"); err == nil || !strings.Contains(err.Error(), "cannot assign to 'x'") {
		t.Fatalf("expected immutability error, got %v", err)
	}

	// Closure captures old x before redeclaration; redeclaring x := x + 5 reads old x
	if _, _, err := s.Eval("fn oldX() => x"); err != nil {
		t.Fatalf("fn oldX: %v", err)
	}
	if _, _, err := s.Eval("x := x + 5"); err != nil {
		t.Fatalf("redeclare x: %v", err)
	}
	val, _, err = s.Eval("[oldX(), x]")
	if err != nil || val.String() != "[10, 15]" {
		t.Fatalf("[oldX(), x]: val=%v err=%v", val, err)
	}

	// Mutable var binding can be mutated across turns and inside functions
	if _, _, err := s.Eval("var count := 0"); err != nil {
		t.Fatalf("var count: %v", err)
	}
	if _, _, err := s.Eval("fn inc() { count += 1\n return count }"); err != nil {
		t.Fatalf("fn inc: %v", err)
	}
	val, _, err = s.Eval("[inc(), inc(), count]")
	if err != nil || val.String() != "[1, 2, 2]" {
		t.Fatalf("inc: val=%v err=%v", val, err)
	}
}

func TestSessionAttachToLoadedEngine(t *testing.T) {
	eng := evie.New()
	defer eng.Close()

	_, err := eng.Eval(`package main
		var score := 40
		fn boost(n) {
			score += n
			return score
		}`)
	if err != nil {
		t.Fatalf("Eval package main: %v", err)
	}

	// Opening a session on an engine with a loaded main package attaches to it
	s := eng.NewSession()
	val, hasVal, err := s.Eval("boost(2)")
	if err != nil || !hasVal || val.String() != "42" {
		t.Fatalf("boost(2): val=%v hasVal=%v err=%v", val, hasVal, err)
	}
	val, _, err = s.Eval("score")
	if err != nil || val.String() != "42" {
		t.Fatalf("score: val=%v err=%v", val, err)
	}
}

func TestSessionFunctionsClassesAndAsync(t *testing.T) {
	eng := evie.New()
	defer eng.Close()
	s := eng.NewSession()

	// Recursive function defined across lines
	if _, _, err := s.Eval(`fn fib(n) {
		if n < 2 { return n }
		return fib(n - 1) + fib(n - 2)
	}`); err != nil {
		t.Fatalf("define fib: %v", err)
	}
	val, _, err := s.Eval("fib(10)")
	if err != nil || val.String() != "55" {
		t.Fatalf("fib(10): val=%v err=%v", val, err)
	}

	// Classes and cross-turn inheritance with super
	if _, _, err := s.Eval(`class Animal(name) {
		fn speak() => this.name
	}`); err != nil {
		t.Fatalf("define Animal: %v", err)
	}
	if _, _, err := s.Eval(`class Dog(name) : Animal {
		fn speak() => super.speak() + "!"
	}`); err != nil {
		t.Fatalf("define Dog: %v", err)
	}
	val, _, err = s.Eval(`Dog("Rex").speak()`)
	if err != nil || val.String() != "Rex!" {
		t.Fatalf("Dog.speak: val=%v err=%v", val, err)
	}

	// Async task created on one turn and awaited on the next
	if _, _, err := s.Eval(`t := go (fn() => 21 * 2)()`); err != nil {
		t.Fatalf("spawn task: %v", err)
	}
	val, _, err = s.Eval("await t")
	if err != nil || val.String() != "42" {
		t.Fatalf("await t: val=%v err=%v", val, err)
	}
}

func TestSessionRollbackOnError(t *testing.T) {
	eng := evie.New()
	defer eng.Close()
	s := eng.NewSession()

	if _, _, err := s.Eval("a := 1"); err != nil {
		t.Fatalf("decl a: %v", err)
	}

	// Compile error during redeclaration must preserve old `a` and not leak `b`
	if _, _, err := s.Eval("b := 2\na := unknownSym"); err == nil {
		t.Fatal("expected compile error")
	}
	val, _, err := s.Eval("a")
	if err != nil || val.String() != "1" {
		t.Fatalf("a after compile error: val=%v err=%v", val, err)
	}
	if _, _, err := s.Eval("b"); err == nil {
		t.Fatal("expected b to be rolled back after compile error")
	}

	// Runtime error during declaration must roll back newly declared symbols too
	if _, _, err := s.Eval(`fn boom() { throw error("nope") }`); err != nil {
		t.Fatalf("define boom: %v", err)
	}
	if _, _, err := s.Eval("a := boom()"); err == nil {
		t.Fatal("expected runtime error")
	}
	val, _, err = s.Eval("a")
	if err != nil || val.String() != "1" {
		t.Fatalf("a after runtime error: val=%v err=%v", val, err)
	}
}

func TestSessionAutoImportAndFSImport(t *testing.T) {
	var stdout bytes.Buffer
	fsys := fstest.MapFS{
		"util.ev": &fstest.MapFile{
			Data: []byte(`package util
				fn triple(n) => n * 3`),
		},
	}

	eng := evie.New(evie.Options{
		FS:     fsys,
		Stdlib: []string{"io", "math"},
		Stdout: &stdout,
	})
	defer eng.Close()
	s := eng.NewSession()

	// Allowed stdlib packages auto-import on first use
	val, _, err := s.Eval("math.sqrt(9)")
	if err != nil || val.String() != "3" {
		t.Fatalf("math.sqrt(9): val=%v err=%v", val, err)
	}
	if _, _, err := s.Eval(`io.println("hi session")`); err != nil {
		t.Fatalf("io.println: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "hi session" {
		t.Fatalf("stdout = %q, want %q", stdout.String(), "hi session")
	}

	// Disallowed stdlib package is NOT auto-imported
	if _, _, err := s.Eval(`strings.upper("hi")`); err == nil {
		t.Fatal("expected strings to be blocked by Options.Stdlib")
	}

	// Explicit aliased import from virtual FS
	if _, err := s.Import("./util", "u"); err != nil {
		t.Fatalf("Import ./util: %v", err)
	}
	val, _, err = s.Eval("u.triple(7)")
	if err != nil || val.String() != "21" {
		t.Fatalf("u.triple(7): val=%v err=%v", val, err)
	}
}

func TestSessionInterruptKeepsSessionAlive(t *testing.T) {
	eng := evie.New(evie.Options{NoStdlib: true})
	defer eng.Close()
	s := eng.NewSession()

	if _, _, err := s.Eval("saved := 123"); err != nil {
		t.Fatalf("saved: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, _, err := s.EvalContext(ctx, "while true {}")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}

	// Session remains usable and keeps earlier bindings
	val, _, err := s.Eval("saved + 1")
	if err != nil || val.String() != "124" {
		t.Fatalf("after timeout: val=%v err=%v", val, err)
	}
}
