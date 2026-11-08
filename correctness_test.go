package evie_test

import (
	"math"
	"strings"
	"testing"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

func TestCrossTypeEqualityNeverCorruptsOrLies(t *testing.T) {
	code := `package main
	struct Box(v) {}
	fn f() => 1
	fn main() {
		checks := [
			false == nil,
			nil == false,
			false == "abc",
			"abc" == false,
			false == 0,
			0 == false,
			0 == nil,
			nil == 0,
			0 == "abc",
			"abc" == 0,
			"abc" == nil,
			nil == "abc",
			true == f,
			f == true,
			Box(1) == 0,
			0 == Box(1),
			"" == false,
			"" == 0,
			"" == nil,
		]
		for c in checks {
			if c {
				throw error("unexpected cross-type equality")
			}
		}
		if !(0 == 0 && false == false && nil == nil && "a" == "a" && 0 != nil && "abc" != false) {
			throw error("same-type equality or inequality broke")
		}
		return true
	}`
	val, err := runCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b, ok := val.AsBool(); !ok || !b {
		t.Fatalf("expected true, got %v", val)
	}

	// Direct Go-level Value.Equals check including host value vs denormal float matching kindHost
	type opaque struct{ id int }
	ht := vm.NewHostType[*opaque]("Phase1Opaque")
	hv := ht.Wrap(&opaque{id: 1})
	denormal := vm.BoxNumber(math.Float64frombits(13))
	if hv.Equals(denormal) || denormal.Equals(hv) {
		t.Fatal("host value must not equal denormal float")
	}
	if vm.BoxString("x").Equals(vm.Value{}) || vm.BoxString("x").Equals(vm.BoxBool(false)) || vm.BoxString("x").Equals(vm.BoxNumber(0)) {
		t.Fatal("string must not equal nil/false/0")
	}
}

func TestTruthinessOfAllTypes(t *testing.T) {
	code := `package main imports("math")
	struct User(name) {
		fn hi() => this.name
	}
	fn main() {
		u := User("Alice")
		e := error("boom")
		bm := u.hi
		sbm := "hi".len
		if !u { throw error("struct instance should be truthy") }
		if !e { throw error("error should be truthy") }
		if !bm { throw error("bound method should be truthy") }
		if !sbm { throw error("builtin bound method should be truthy") }
		if !math.sqrt { throw error("GoFunc should be truthy") }
		if !User { throw error("struct type should be truthy") }
		if !string { throw error("builtin type should be truthy") }
		if !math { throw error("package should be truthy") }

		if nil || false || 0 || "" || [] || {} || buffer() {
			throw error("falsy values became truthy")
		}
		return true
	}`
	val, err := runCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b, ok := val.AsBool(); !ok || !b {
		t.Fatalf("expected true, got %v", val)
	}
}

func TestThreeLevelClosureCapture(t *testing.T) {
	code := `package main
	fn outer() {
		a := "A"
		b := "B"
		c := "C"
		fn mid() {
			// capture c first (slot 0), then a (slot 1), then b (slot 2)
			keep := c
			fn innerOnlyA() => a
			fn innerReordered() => b + a + c
			return keep + ":" + innerOnlyA() + ":" + innerReordered()
		}
		return mid()
	}
	fn main() => outer()`
	val, err := runCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s, _ := val.AsString(); s != "C:A:BAC" {
		t.Fatalf("expected C:A:BAC, got %q", s)
	}
}

func TestLocalStructMethodCapturesAndDynamicParents(t *testing.T) {
	code := `package main
	struct Base1(x) {
		fn tag() => "B1:" + string(this.x)
	}
	struct Base2(x) {
		fn tag() => "B2:" + string(this.x)
	}
	fn makeGreeter(prefix) {
		struct Greeter(name) {
			fn greet() => prefix + " " + this.name
		}
		return Greeter
	}
	fn makeSub(Parent, suffix) {
		struct Sub(x) : Parent {
			fn tag() => super.tag() + suffix
		}
		return Sub
	}
	fn main() {
		G1 := makeGreeter("Hello")
		G2 := makeGreeter("Hi")
		g1 := G1("Alice")
		g2 := G2("Bob")

		S1 := makeSub(Base1, "!")
		S2 := makeSub(Base2, "?")
		s1 := S1(1)
		s2 := S2(2)

		return g1.greet() + "|" + g2.greet() + "|" + s1.tag() + "|" + s2.tag()
	}`
	val, err := runCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Hello Alice|Hi Bob|B1:1!|B2:2?"
	if s, _ := val.AsString(); s != want {
		t.Fatalf("expected %q, got %q", want, s)
	}
}

func TestSuperMethodArityChecked(t *testing.T) {
	tooFew := `package main
	struct A(x) { fn add(a, b) => this.x + a + b }
	struct B(x) : A { fn run() => super.add(1) }
	fn main() => B(10).run()`
	if _, err := runCode(tooFew); err == nil || !strings.Contains(err.Error(), "takes 2 arguments but 1 was supplied") {
		t.Fatalf("expected arity error for too few args, got %v", err)
	}

	tooMany := `package main
	struct A(x) { fn add(a) => this.x + a }
	struct B(x) : A { fn run() => super.add(1, 2, 3) }
	fn main() => B(10).run()`
	if _, err := runCode(tooMany); err == nil || !strings.Contains(err.Error(), "takes 1 argument but 3 were supplied") {
		t.Fatalf("expected arity error for too many args, got %v", err)
	}
}

func TestEvalCompileErrorDoesNotCorruptEngine(t *testing.T) {
	eng := evie.New()
	if _, err := eng.Eval(`fn a() { fn b() { return undefinedName } }`); err == nil {
		t.Fatal("expected compile error")
	}
	val, err := eng.Eval(`x := 20
	return x + 22`)
	if err != nil {
		t.Fatalf("subsequent Eval failed after compile error: %v", err)
	}
	if n, _ := val.AsFloat64(); n != 42 {
		t.Fatalf("expected 42, got %v", val)
	}
}

func TestHostTypeWithoutConstructorDoesNotPanic(t *testing.T) {
	type handle struct{ id int }
	ht := vm.NewHostType[*handle]("Phase1Handle")
	boxed := ht.Box()
	if boxed.IsCallable() {
		t.Fatal("HostType without constructor should not report IsCallable")
	}
	if boxed.AcceptsArgs() {
		t.Fatal("HostType without constructor should not report AcceptsArgs")
	}
	if _, err := boxed.Call(); err == nil || !strings.Contains(err.Error(), "cannot be called as a constructor") {
		t.Fatalf("expected constructor error, got %v", err)
	}

	eng := evie.New()
	eng.SetStatic("Handle", boxed)
	if _, err := eng.Eval(`return Handle()`); err == nil || !strings.Contains(err.Error(), "cannot be called as a constructor") {
		t.Fatalf("static call to non-constructible HostType: got %v", err)
	}
	if _, err := eng.Eval(`h := Handle
	return h()`); err == nil || !strings.Contains(err.Error(), "cannot be called as a constructor") {
		t.Fatalf("dynamic call to non-constructible HostType: got %v", err)
	}
}

func TestEngineAndValueCallOnNativesAndBuiltins(t *testing.T) {
	eng := evie.New()
	if _, err := eng.Eval(`package main imports("math")`); err != nil {
		t.Fatal(err)
	}

	v, err := eng.Call("math.sqrt", 16)
	if err != nil {
		t.Fatalf("eng.Call(math.sqrt): %v", err)
	}
	if n, _ := v.AsFloat64(); n != 4 {
		t.Fatalf("expected 4, got %v", v)
	}

	v, err = eng.Call("string", 42)
	if err != nil {
		t.Fatalf("eng.Call(string): %v", err)
	}
	if s, _ := v.AsString(); s != "42" {
		t.Fatalf("expected \"42\", got %v", v)
	}

	sqrtVal, ok := eng.Lookup("math.sqrt")
	if !ok {
		t.Fatal("math.sqrt not found")
	}
	v, err = sqrtVal.Call(vm.BoxNumber(9))
	if err != nil {
		t.Fatalf("Value.Call(math.sqrt): %v", err)
	}
	if n, _ := v.AsFloat64(); n != 3 {
		t.Fatalf("expected 3, got %v", v)
	}

	strType, ok := eng.Lookup("string")
	if !ok {
		t.Fatal("string not found")
	}
	v, err = strType.Call(vm.BoxNumber(7))
	if err != nil {
		t.Fatalf("Value.Call(string): %v", err)
	}
	if s, _ := v.AsString(); s != "7" {
		t.Fatalf("expected \"7\", got %v", v)
	}
}

func TestCallIntRejectsInfAndMethodErrorIndex(t *testing.T) {
	cases := []struct {
		code string
		want string
	}{
		{`package main imports("math", "strings")
		fn main() => strings.repeat("a", math.inf)`, "must be an integer"},
		{`package main imports("math", "strings")
		fn main() => strings.repeat("a", -math.inf)`, "must be an integer"},
		{`package main imports("math", "strings")
		fn main() => strings.repeat("a", math.nan)`, "must be an integer"},
		{`package main imports("math", "strings")
		fn main() => strings.repeat("a", math.pow(10, 30))`, "must be an integer"},
		{`return "hello".contains(123)`, "contains: argument 1 must be a string, got number"},
	}
	for _, tc := range cases {
		_, err := runCode(tc.code)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want %q", tc.code, err, tc.want)
		}
	}
}

func TestPrivatePackageImportsNotAccessible(t *testing.T) {
	for _, code := range []string{
		`package main imports("helper")
		fn main() => helper.math()`,
		`package main imports("helper")
		fn main() { helper.math = 1 }`,
		`package main imports("helper")
		fn main() {
			h := helper
			h.math()
		}`,
		`package main imports("helper")
		fn main() {
			h := helper
			h.math = 1
		}`,
		`package main imports("helper")
		fn main() {
			h := helper
			h.math += 1
		}`,
	} {
		eng := evie.New()
		if _, err := eng.Load("helper.ev", []byte(`package helper imports("math")
		fn val() => 1`)); err != nil {
			t.Fatal(err)
		}
		_, err := eng.Eval(code)
		if err == nil {
			_, err = eng.Call("main.main")
		}
		if err == nil {
			t.Errorf("expected error accessing private import via %s", code)
		}
	}
}

func TestLexerEscapesTemplatesMultilineAndComments(t *testing.T) {
	// Strings and templates ending in escaped backslashes
	val, err := runCode(`return "\\" + "|" + "C:\\Users\\" + "|" + "a\\\"b"`)
	if err != nil {
		t.Fatalf("backslash string failed: %v", err)
	}
	if s, _ := val.AsString(); s != `\|C:\Users\|a\"b` {
		t.Fatalf("unexpected backslash string result: %q", s)
	}

	val, err = runCode("return `foo\\\\` + \"|\" + `bar\\\\{1 + 2}` + \"|\" + `baz\\{1}`")
	if err != nil {
		t.Fatalf("backslash template failed: %v", err)
	}
	if s, _ := val.AsString(); s != `foo\|bar\3|baz{1}` {
		t.Fatalf("unexpected backslash template result: %q", s)
	}

	// Source starting directly with a template literal
	if _, err := evie.Eval("`hello {1 + 2}`"); err != nil {
		t.Fatalf("template at start of source failed: %v", err)
	}

	// Nested template literals
	val, err = runCode("return `outer {`inner {1 + 2}`}`")
	if err != nil {
		t.Fatalf("nested template failed: %v", err)
	}
	if s, _ := val.AsString(); s != "outer inner 3" {
		t.Fatalf("expected %q, got %q", "outer inner 3", s)
	}

	// Byte 0x03 inside string must not truncate source
	val, err = runCode("s := \"a\x03b\"\nreturn s.len()")
	if err != nil {
		t.Fatalf("0x03 byte in string failed: %v", err)
	}
	if n, _ := val.AsFloat64(); n != 3 {
		t.Fatalf("expected 3, got %v", val)
	}

	// Multiline string diagnostic must not panic when formatted
	_, err = evie.Eval("x := \"line1\nline2\" - 1")
	if err == nil {
		t.Fatal("expected error subtracting from multiline string")
	}
	if formatted := evie.Format(err, false); !strings.Contains(formatted, "cannot apply '-'") {
		t.Fatalf("unexpected formatted multiline error: %s", formatted)
	}

	_, err = evie.Eval("x := \"line1\nline2")
	if err == nil {
		t.Fatal("expected error for unterminated multiline string")
	}
	if formatted := evie.Format(err, false); !strings.Contains(formatted, "never closed") {
		t.Fatalf("unexpected formatted unterminated multiline error: %s", formatted)
	}

	// Unterminated block comment
	if _, err := evie.Eval("/* unclosed comment"); err == nil || !strings.Contains(err.Error(), "block comment is never closed") {
		t.Fatalf("expected unclosed block comment error, got %v", err)
	}
}

func TestParserOperatorAndPrecedenceFixes(t *testing.T) {
	// String literal must NOT parse as binary operator
	for _, bad := range []string{`return (1 "+" 2)`, `return (1 "." 2)`, `return (1 "is" 2)`} {
		if val, err := evie.Eval(bad); err == nil {
			t.Fatalf("%s unexpectedly evaluated to %v", bad, val)
		}
	}

	// Unary ! binds tighter than *
	if _, err := runCode(`return !0 * 2`); err == nil || !strings.Contains(err.Error(), "cannot apply '*' to a bool and a number") {
		t.Fatalf("expected bool * number error for !0 * 2, got %v", err)
	}

	// Unary - binds tighter than * and /
	val, err := runCode(`x := 3
	return -x * 2`)
	if err != nil {
		t.Fatalf("-x * 2 failed: %v", err)
	}
	if n, _ := val.AsFloat64(); n != -6 {
		t.Fatalf("expected -6, got %v", val)
	}

	// await, await.*, go, fn expression precedence & chaining
	code := `package main
	fn double(n) => n * 2
	fn main() {
		t1 := go double(5)
		t2 := go double(7)
		a := await t1 + await t2
		b := await.all(go double(2), go double(3))[1]
		c := await.timeout(go double(4), 1000) + 1
		d := fn(x) { return x * 3 }(7)
		e := await go fn() { return 10 }()
		return [a, b, c, d, e]
	}`
	val, err = runCode(code)
	if err != nil {
		t.Fatalf("async/fn precedence & chaining failed: %v", err)
	}
	if val.String() != "[24, 6, 9, 21, 10]" {
		t.Fatalf("expected [24, 6, 9, 21, 10], got %s", val.String())
	}
}
