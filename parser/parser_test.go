package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
