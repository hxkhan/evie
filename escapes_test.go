package evie_test

import (
	"strings"
	"testing"
)

func expectString(t *testing.T, expr, want string) {
	t.Helper()
	got, err := runCode("package main\nfn main() {\n\treturn " + expr + "\n}")
	if err != nil {
		t.Errorf("%s: %v", expr, err)
	} else if got.String() != want {
		t.Errorf("%s = %q, want %q", expr, got.String(), want)
	}
}

func expectFailure(t *testing.T, expr, wantInError string) {
	t.Helper()
	_, err := runCode("package main\nfn main() {\n\treturn " + expr + "\n}")
	if err == nil || !strings.Contains(err.Error(), wantInError) {
		t.Errorf("%s: want an error mentioning %q, got %v", expr, wantInError, err)
	}
}

func TestStringEscapes(t *testing.T) {
	for expr, want := range map[string]string{
		`"a\nb".len()`:        "3",
		`"tab\there"`:         "tab\there",
		`"say \"hi\""`:        `say "hi"`,
		`"back\\slash".len()`: "10",
		"`{1 + 1}% done`":     "2% done",
		"`50%`":               "50%",
		"`a \\{b\\} \\` c`":   "a {b} ` c",
		"`line\\n{2}`.len()":  "6",
	} {
		expectString(t, expr, want)
	}
	expectFailure(t, `"C:\Users"`, `unknown escape '\U'`)
}
