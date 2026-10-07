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
		"struct":                               "source ends here",
		"fn main() { `template {1 }":           "expected",
		"fn main() { x.if }":                   "",
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
		if filepath.Base(file) == "types.ev" {
			continue // a syntax sketch, not a program
		}
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
