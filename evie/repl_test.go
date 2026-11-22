package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIREPLLoop(t *testing.T) {
	script := strings.Join([]string{
		"x := 20",
		"fn double(n) {",
		"  return n * 2",
		"}",
		"double(x)",
		"_ + 2",
		":vars",
		":dump",
		":reset",
		"x",
		":help",
		":quit",
	}, "\n") + "\n"

	var out, errOut bytes.Buffer
	runREPLLoop(strings.NewReader(script), &out, &errOut, replConfig{
		inline:      true,
		interactive: false,
		colors:      false,
	}, nil, nil, nil)

	gotOut := out.String()
	gotErr := errOut.String()

	for _, want := range []string{
		"40\n",
		"42\n",
		"const x = 20",
		"fn double",
		"fn double(n)",
		"session reset",
		"REPL commands:",
	} {
		if !strings.Contains(gotOut, want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, gotOut)
		}
	}

	if !strings.Contains(gotErr, "undefined") && !strings.Contains(gotErr, "'x'") {
		t.Errorf("expected error for `x` after :reset, got stderr:\n%s", gotErr)
	}
}

func TestIsIncompleteInput(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{"1 + 2", false},
		{"fn add(a, b) => a + b", false},
		{"fn add(a, b) {", true},
		{"fn add(a, b) {\n  return a + b\n}", false},
		{"arr := [1, 2,", true},
		{"arr := [1, 2, 3]", false},
		{"fn f(x) =>", true},
		{"fn f(x) => // comment", true},
		{"fn f(x) => x // {", false},
		{`s := "unclosed { inside string"`, false},
		{"t := `hello\nworld", true},
		{"t := `hello\nworld`", false},
		{"t := `value = {1 + ", true},
		{"t := `value = {1 + 2}`", false},
		{"{ ]", false},
	}
	for _, tc := range cases {
		if got := isIncompleteInput(tc.src); got != tc.want {
			t.Errorf("isIncompleteInput(%q) = %v, want %v", tc.src, got, tc.want)
		}
	}
}

// -e code is a program when it declares a package, comments before the header included
func TestHasPackageHeader(t *testing.T) {
	cases := map[string]bool{
		"package main\nfn main() => 1":           true,
		"// note\npackage main\nfn main() => 1":  true,
		"/* note */ package main fn main() => 1": true,
		"1 + 2":                                  false,
		"packages := 1":                          false,
	}
	for code, want := range cases {
		if got := hasPackageHeader(code); got != want {
			t.Errorf("hasPackageHeader(%q) = %v, want %v", code, got, want)
		}
	}
}
