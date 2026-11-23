package parser_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/parser"
)

// malformed input must come back as an error, never as a panic or a silently truncated program
func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"@":                                    "invalid token",
		"package main\nfn main() { x := ) }":   "unexpected ')'",
		"package main\nfn main() { return 1 +": "unexpected end of source",
		"package main\nfn main() {":            "source ends here",
		"fn main() { x := [1, 2":               "source ends here",
		"fn main() { x := { a: 1":              "source ends here",
		"fn main() { f(1, 2":                   "source ends here",
		"x := \"unterminated":                  "never closed",
		"fn main() { if x }":                   "expected '{'",
		"fn main() { for x }":                  "expected 'in'",
		"fn main() { switch x { boom } }":      "expected 'case'",
		"class":                                "source ends here",
		"fn main() { `template {1 }":           "expected",
		"fn main() { x.if }":                   "",
		// a statement where a value is needed
		"fn main() { return var x := 1 }":                             "expected a value, found a declaration",
		"fn main() { return x := 1 }":                                 "expected a value, found a declaration",
		"fn main() { var y := 0\n return y = 5 }":                     "expected a value, found an assignment",
		"fn main() { a := [var z := 1] }":                             "expected a value, found a declaration",
		"fn main() { f(x += 1) }":                                     "expected a value, found an assignment",
		"fn main() { while true { x := (break) } }":                   "expected a value, found 'break'",
		"fn main() { x := return 1 }":                                 "expected a value, found 'return'",
		"class C(n) { fn reset() => this.n = 0 }":                     "",
		"fn main() { var n := 0\n inc := fn() => n += 1 }":            "",
		"fn main() { else := 1 }":                                     "keyword",
		"fn main() { in := 1 }":                                       "keyword",
		"fn main() { var switch := 1 }":                               "keyword",
		"fn main() { case := 1 }":                                     "keyword",
		"fn main() { default := 1 }":                                  "keyword",
		"fn if() {}":                                                  "keyword",
		"class while(x) {}":                                           "keyword",
		"fn main() { var class := 1 }":                                "keyword",
		"fn main() { struct := 1\n return struct }":                   "",
		"fn main() { switch 1 { case 1: return\ncase 2: return 2 } }": "",
		"fn main() { switch 1 { case 1: return; case 2: return 2 } }": "",
		"fn main() { return\nx := 1 }":                                "",
		"/* unclosed":                                                 "block comment is never closed",
		"x := \"a\\\\\"":                                              "",
		"x := `a\\\\`":                                                "",
		"`hi {1}`":                                                    "",
		"`a {`b {1}`}`":                                               "",
		"x := await t1 + await t2":                                    "",
		"x := await.all(t1, t2)[0]":                                   "",
		"x := fn(n) { return n }(1)":                                  "",
		"x := 1; y := 2; return x + y":                                "",
		"fn main() { x := if true { 1 } }":                            "expected a value, found an 'if' statement",
		"fn main() { x := switch 1 { case 1: 2 } }":                   "expected a value, found a 'switch' statement",
		"fn main() { x := echo 1 }":                                   "expected a value, found 'echo'",
		strings.Repeat("(", 300) + "1" + strings.Repeat(")", 300):     "nested too deeply",
		// templates are lexed eagerly; this used to overflow the Go stack in the lexer
		"x := " + strings.Repeat("`{", 1_000_000):                           "nested too deeply",
		"fn main() { if a {}" + strings.Repeat(" else if a {}", 300) + " }": "nested too deeply",
		// a malformed number is one token, not a number followed by a name
		"x := 1e5":    "'1e5' is not a valid number",
		"x := 0x10":   "'0x10' is not a valid number",
		"x := 123abc": "'123abc' is not a valid number",
		"x := 1.25":   "",
		// object keys may be named like keywords, as members after '.' may
		"x := {default: 1, in: 2, fn: 3}":                                   "",
		"fn main() { if a {}" + strings.Repeat(" else if a {}", 200) + " }": "",
	}
	for src, want := range cases {
		node, err := parser.Parse([]byte(src))
		if want == "" {
			if err != nil {
				t.Errorf("%q should parse, got %v", src, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%q parsed without error into %v", src, node)
			continue
		}
		if strings.Contains(err.Error(), "internal parser error") {
			t.Errorf("%q crashed the parser: %v", src, err)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %q, want it to mention %q", src, err, want)
		}
	}
}

// a syntax error does not end parsing: each statement's mistake is reported,
// and the skipping in between does not invent errors of its own
func TestParseReportsEveryError(t *testing.T) {
	cases := []struct {
		src  string
		want []string // each error's message, in order
	}{
		{"package main\nfn main() {\n    y := (2\n    z := [1, 2 3]\n    return z\n}",
			[]string{"expected ')', found 'z'", "expected ',' or ']', found number 3"}},
		// a missing comma inside a multi-line object is one error, not one per line
		{"package main\nfn main() {\n    o := {\n        a: 1\n        b: 2\n    }\n    q := )\n}",
			[]string{"expected ',' or '}', found 'b'", "unexpected ')'"}},
		{"package main\nfn main() {\n    f(1, 2\n    var n := 0\n    n := := 1\n}",
			[]string{"expected ',' or ')', found 'var'", "unexpected ':='"}},
		// a broken method does not take the rest of the class with it
		{"package main\nclass C(a) {\n    fn get( => a\n    fn ok() => 1\n}\nfn main() { return 1 + }",
			[]string{"expected a name, found '=>'", "expected a value, found '}'"}},
		{"package main\nfn main() {\n    switch 1 {\n    case 1: x := )\n    case 2: y := 2\n    default: z := (\n    }\n}",
			[]string{"unexpected ')'", "unexpected '}'"}},
		{"package main\nfn main() {\n    s := `a {1 +} b\n    c`\n    t := )\n}",
			[]string{"expected a value, found '}'", "unexpected ')'"}},
		{"x := 1 +; y := ); z := 3", []string{"unexpected ';'", "unexpected ')'"}},
		// a block that is never closed is one error at the end
		{"package main\nfn main() {\n    if true {\n        x := 1\n}\nfn other() {}",
			[]string{"source ends here"}},
		// the package header can be broken too
		{"package\nfn main() { x := ) }", []string{"the package name", "unexpected ')'"}},
	}
	for _, c := range cases {
		_, err := parser.Parse([]byte(c.src))
		var got []string
		var list parser.ErrorList
		var one *parser.Error
		switch {
		case errors.As(err, &list):
			for _, e := range list {
				got = append(got, e.Message)
			}
		case errors.As(err, &one):
			got = []string{one.Message}
		default:
			t.Errorf("%q: got %v, want syntax errors", c.src, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("%q: got %d errors %q, want %d", c.src, len(got), got, len(c.want))
			continue
		}
		for i := range got {
			if !strings.Contains(got[i], c.want[i]) {
				t.Errorf("%q: error %d is %q, want it to mention %q", c.src, i+1, got[i], c.want[i])
			}
		}
	}
}

// the statements around a mistake still parse, and there is a limit to how
// many errors one source reports
func TestParseRecovery(t *testing.T) {
	node, err := parser.Parse([]byte("package main\nfn a() => 1\nfn b() { x := ) }\nfn c() => 3"))
	if err == nil {
		t.Fatal("want an error")
	}
	pack, ok := node.(ast.Package)
	if !ok || len(pack.Code) != 3 {
		t.Fatalf("want all three functions of the package, the broken one without its bad statement, got %#v", node)
	}

	_, err = parser.Parse([]byte(strings.Repeat("x := )\n", 50)))
	var list parser.ErrorList
	if !errors.As(err, &list) || len(list) != 10 {
		t.Fatalf("want 10 errors, got %v", err)
	}
	if !strings.Contains(err.Error(), "(and 9 more)") {
		t.Errorf("the one-line form should count the others: %q", err)
	}
}

// names that declare or name something keep where they are written, and a
// block knows where it ends, for tools such as editors
func TestParsePositions(t *testing.T) {
	src := `package demo imports("io" as out)
class Point(x, y) : Base {
    fn norm(scale, ...rest) {
        for i, v in [x, y] { out.println(i) }
        return {len: scale}.len
    }
}`
	node, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(src, "\n")
	// at reports the text a position covers
	at := func(p diag.Pos) string {
		if !p.Known() {
			return "<unknown>"
		}
		line := lines[p.Line-1]
		return line[p.Column-1 : p.Column-1+max(p.Length, 1)]
	}
	check := func(what string, p diag.Pos, want string) {
		t.Helper()
		if got := at(p); got != want {
			t.Errorf("%s: position covers %q, want %q", what, got, want)
		}
	}

	pack := node.(ast.Package)
	check("package name", pack.NamePos, "demo")
	check("import alias", pack.Imports[0].AliasPos, "out")

	cls := pack.Code[0].(ast.ClassDefinition)
	check("class name", cls.NamePos, "Point")
	check("parent", cls.ParentPos, "Base")
	check("field x", cls.ArgPos[0], "x")
	check("field y", cls.ArgPos[1], "y")
	body := cls.Action.(ast.Block)
	check("class body end", body.End, "}")
	if body.End.Line != 7 {
		t.Errorf("the class body ends on line %d, want 7", body.End.Line)
	}

	fn := body.Code[0].(ast.Fn)
	check("method name", fn.NamePos, "norm")
	check("parameter", fn.ArgPos[0], "scale")
	check("variadic parameter", fn.ArgPos[1], "rest")
	fnBody := fn.Action.(ast.Block)
	if fnBody.End.Line != 6 {
		t.Errorf("the method body ends on line %d, want 6", fnBody.End.Line)
	}

	loop := fnBody.Code[0].(ast.For)
	check("index variable", loop.IndexPos, "i")
	check("item variable", loop.ItemPos, "v")
	call := loop.Action.(ast.Block).Code[0].(ast.Call)
	check("member name", call.Fn.(ast.FieldAccess).RhsPos, "println")

	ret := fnBody.Code[1].(ast.Return)
	access := ret.Value.(ast.FieldAccess)
	check("member after an object", access.RhsPos, "len")
	check("object key", access.Lhs.(ast.Object).Fields[0].KeyPos, "len")
}

func TestParseExamples(t *testing.T) {
	files, _ := filepath.Glob("../examples/*.ev")
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(src); err != nil {
			t.Errorf("%s: %v", file, err)
		}
	}
}

func FuzzParse(f *testing.F) {
	files, _ := filepath.Glob("../examples/*.ev")
	for _, file := range files {
		if src, err := os.ReadFile(file); err == nil {
			f.Add(string(src))
		}
	}
	f.Add("package main imports(\"io\")\nfn main() { io.println(`a {1 + 2} b`) }")
	f.Add("x := [1, {a: 2}, fn(y) => y * 2, \"s\"[0]?, go f(), await.all(t)]")
	f.Fuzz(func(t *testing.T, src string) {
		_, err := parser.Parse([]byte(src))
		if err != nil && strings.Contains(err.Error(), "internal parser error") {
			t.Fatalf("parser crashed on %q: %v", src, err)
		}
	})
}

// the parsed program does not share memory with the caller's buffer, so the
// caller may reuse it
func TestParseDoesNotAliasInput(t *testing.T) {
	buf := []byte(`hello := "world"`)
	node, err := parser.Parse(buf)
	if err != nil {
		t.Fatal(err)
	}
	before := node.(interface{ String() string }).String()
	copy(buf, `HELLO := "WORLD"`)
	if after := node.(interface{ String() string }).String(); after != before {
		t.Fatalf("changing the input buffer changed the parsed program: %q became %q", before, after)
	}
}
