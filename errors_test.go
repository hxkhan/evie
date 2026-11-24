package evie_test

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

// every failure the runtime and the standard library raise has a kind, so a
// script can tell them apart without reading messages
func TestErrorKinds(t *testing.T) {
	cases := []struct{ code, kind string }{
		{`1 + "x"`, "type"},
		{`fn(a) => a`, ""}, // a placeholder replaced below
		{`number("two")`, "value"},
		{`[1, 2][5]`, "range"},
		{`{}.missing`, "member"},
		{`fs.readText("missing.txt")`, "notFound"},
		{`fs.writeFile("x.txt", "hi")`, "permission"},
		{`await.timeout(go time.wait(200), 5)`, "timeout"},
		{`json.decode("{bad")`, "syntax"},
		{`time.parse("not a time")`, "syntax"},
		{`math.clamp(1, 5, 2)`, "value"},
		{`error("plain")`, "error"},
	}
	cases[1] = struct{ code, kind string }{`(fn(a) => a)(1, 2)`, "type"}
	for _, c := range cases {
		code := fmt.Sprintf(`package main imports("fs", "json", "time", "math")
fn main() {
    e := catch(%s)
    if e is error {
        return e.kind
    }
    return "no error"
}`, c.code)
		eng := evie.New(evie.Options{FS: fstest.MapFS{}})
		b, err := evie.ParseSource("", []byte(code))
		if err != nil {
			t.Fatal(err)
		}
		v, err := eng.RunBundle(t.Context(), b)
		eng.Close()
		if err != nil {
			t.Errorf("%s: %v", c.code, err)
			continue
		}
		if s, _ := v.AsString(); s != c.kind {
			t.Errorf("%s: kind %q, want %q", c.code, s, c.kind)
		}
	}
}

func TestErrorFieldsAndCause(t *testing.T) {
	v := run(t, `
	inner := catch(1 + "x")
	e := error("cannot add", {kind: "config", path: "app.json", cause: inner})
	return [e.message, e.kind, e.path, e.cause.kind, e.cause is error, e.code?, error("x").cause]`)
	want := `["cannot add", "config", "app.json", "type", true, false, nil]`
	if v.String() != want {
		t.Errorf("got  %s\nwant %s", v, want)
	}
}

// a thrown value that is not an error keeps its data
func TestThrowingValues(t *testing.T) {
	v := run(t, `
	a := catch { throw {message: "not found", kind: "http", code: 404} }
	b := catch { throw "plain" }
	c := catch { throw 42 }
	return [a.message, a.kind, a.code, b.message, b.kind, c.message, c.value]`)
	want := `["not found", "http", 404, "plain", "error", "42", 42]`
	if v.String() != want {
		t.Errorf("got  %s\nwant %s", v, want)
	}
}

func TestErrorMisuse(t *testing.T) {
	for code, want := range map[string]string{
		`error("x", {kind: 1})`:            "kind must be a string",
		`error("x", {line: 3})`:            "'line' is where it was thrown",
		`error("x", {message: "y"})`:       "first argument",
		`error("x", 1)`:                    "object of fields",
		`error("x", {}, 3)`:                "takes a message and an object of fields",
		`catch { throw {trace: []} }.kind`: "",
	} {
		v, err := runCode(code)
		if want == "" {
			// a thrown object with a field an error cannot take is a type error
			if s, _ := v.AsString(); err != nil || s != "type" {
				t.Errorf("%s: got %v, %v", code, v, err)
			}
			continue
		}
		var e *vm.Error
		if !errors.As(err, &e) || e.Kind() != vm.KindType || !strings.Contains(e.Message, want) {
			t.Errorf("%s: got %v, want a type error saying %q", code, err, want)
		}
	}
}

// the catch block guards several statements, and a return inside it returns
// from the function
func TestCatchBlock(t *testing.T) {
	v := run(t, `
	fn first(xs) {
	    catch {
	        if xs.len() > 0 {
	            return xs[0]
	        }
	    }
	    return "empty"
	}
	e := catch {
	    a := 1
	    b := a + "x"
	}
	return [first([7]), first([]), e.kind, catch { a := 1 }]`)
	want := `[7, "empty", "type", nil]`
	if v.String() != want {
		t.Errorf("got  %s\nwant %s", v, want)
	}
}

// a native gives errors kinds and fields with vm.Errorf and With; plain Go
// errors take their kind from what they wrap; the host sees kinds, fields and
// causes, and errors.Is sees through a cause a script gave
func TestErrorsAcrossTheHost(t *testing.T) {
	pkg := vm.NewPackage("store")
	pkg.Func("get", 1, func(c vm.Call) (vm.Value, error) {
		key, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		if key == "missing" {
			return vm.Value{}, fmt.Errorf("get %s: %w", key, iofs.ErrNotExist)
		}
		return vm.Value{}, vm.Errorf("locked", "'%s' is locked", key).With("key", vm.BoxString(key))
	})
	eng := evie.New(evie.Options{Packages: []*vm.Package{pkg}})
	defer eng.Close()
	_, err := eng.Eval(`package main imports("store")
fn kinds() {
    a := catch(store.get("missing"))
    b := catch(store.get("door"))
    return [a.kind, b.kind, b.key, b.message]
}
fn wrapped() {
    throw error("cannot open", {cause: catch(store.get("missing")), attempt: 2})
}`)
	if err != nil {
		t.Fatal(err)
	}
	v, err := eng.Call("main.kinds")
	if err != nil {
		t.Fatal(err)
	}
	if want := `["notFound", "locked", "door", "'door' is locked"]`; v.String() != want {
		t.Errorf("got  %s\nwant %s", v, want)
	}

	_, err = eng.Call("main.wrapped")
	var e *vm.Error
	if !errors.As(err, &e) {
		t.Fatalf("want a *vm.Error, got %T %v", err, err)
	}
	attempt, _ := e.Field("attempt")
	if e.Kind() != vm.KindError || attempt.String() != "2" || e.CauseValue().IsNil() {
		t.Errorf("kind %q, attempt %v, cause %v", e.Kind(), attempt, e.CauseValue())
	}
	if !errors.Is(err, iofs.ErrNotExist) {
		t.Error("errors.Is does not see the cause the script gave")
	}
}
