package evie_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/diag"
)

// render loads a program as example.ev, runs its main and returns the error
// rendered the way the CLI prints it, without colors
func render(t *testing.T, src string) string {
	t.Helper()
	eng := evie.New()
	var err error
	if _, err = eng.Load("example.ev", []byte(src)); err == nil {
		_, err = eng.Call("main.main")
	}
	if err == nil {
		t.Fatalf("expected an error from:\n%s", src)
	}
	var reporter interface{ Diagnostic() *diag.Report }
	if !errors.As(err, &reporter) {
		t.Fatalf("error carries no report: %T %v", err, err)
	}
	return reporter.Diagnostic().Render(diag.Palette{})
}

func expectRendering(t *testing.T, src string, want string) {
	t.Helper()
	got := render(t, src)
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("rendering differs\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderedSyntaxError(t *testing.T) {
	expectRendering(t, `package main imports("io")

fn main() {
    total := compute(1, 2
    io.println(total)
}
`, `error: expected ',' or ')', found 'io'
 --> example.ev:5:5
  |
5 |     io.println(total)
  |     ^^
  |
4 |     total := compute(1, 2
  |                     - this '(' opens the call
  |
`)
}

func TestRenderedRuntimeErrorWithTrace(t *testing.T) {
	expectRendering(t, `package main

fn average(values) {
    var sum := 0
    for v in values {
        sum += v
    }
    return sum / values.len()
}

fn report(scores) => average(scores)

fn main() {
    report([90, 85, "n/a"])
}
`, `error: cannot apply '+' to a number and a string
 --> example.ev:6:9
  |
6 |         sum += v
  |         ^^^^^^^^ number + string
  |
  = help: convert the other side with string(...) to concatenate, or with number(...) to add
  = trace: average at example.ev:6:9
           report at example.ev:11:22
           main at example.ev:14:5
`)
}

func TestRenderedSuggestions(t *testing.T) {
	expectRendering(t, `package main

fn main() {
    greeting := "hello"
    return greting
}
`, `error: cannot find 'greting' in this scope
 --> example.ev:5:12
  |
5 |     return greting
  |            ^^^^^^^
  |
  = help: a binding with a similar name exists: 'greeting'
`)

	expectRendering(t, `package main

struct Point(x, y) {
    fn length() => this.x * this.x + this.y * this.y
}

fn main() {
    p := Point(3, 4)
    return p.lenght()
}
`, `error: no member 'lenght' on a Point
 --> example.ev:9:12
  |
9 |     return p.lenght()
  |            ^^^^^^^^
  |
  = help: a Point has a member with a similar name: 'length'
  = trace: main at example.ev:9:12
`)
}

func TestRenderedArityError(t *testing.T) {
	expectRendering(t, `package main

fn clamp(x, lo, hi) {
    if x < lo { return lo }
    if x > hi { return hi }
    return x
}

fn main() {
    return clamp(15, 10)
}
`, `error: function 'clamp' takes 3 arguments but 2 were supplied
  --> example.ev:10:12
   |
10 |     return clamp(15, 10)
   |            ^^^^^^^^^^^^^ 2 arguments passed here
   |
 3 | fn clamp(x, lo, hi) {
   | -------- declared here
   |
`)
}

// a caught error tells the script where it happened
func TestErrorValuesCarryPositions(t *testing.T) {
	eng := evie.New()
	_, err := eng.Load("where.ev", []byte(`package main

fn fail() {
    throw error("boom")
}

fn main() {
    e := catch(fail())
    return [e.message, e.file, e.line, e.column, e.trace.len(), e.trace[0], e.trace[1]]
}
`))
	if err != nil {
		t.Fatal(err)
	}
	val, err := eng.Call("main.main")
	if err != nil {
		t.Fatal(err)
	}
	want := `["boom", "where.ev", 4, 5, 2, "fail at where.ev:4:5", "main at where.ev:8:16"]`
	if val.String() != want {
		t.Fatalf("got  %s\nwant %s", val, want)
	}
}

// a native's error is located at the call that made it
func TestNativeErrorsAreLocated(t *testing.T) {
	got := render(t, `package main imports("fs")

fn main() {
    fs.readFile("/no/such/file.txt")
}
`)
	for _, want := range []string{"example.ev:4:5", `fs.readFile("/no/such/file.txt")`, "trace: main at example.ev:4:5"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendering lacks %q:\n%s", want, got)
		}
	}
}
