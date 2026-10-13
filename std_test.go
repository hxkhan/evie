package evie_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stdCase evaluates expr in a script that imports the standard library and
// compares the printed result
func stdCase(t *testing.T, expr, want string) {
	t.Helper()
	script := fmt.Sprintf("package main imports(\"math\", \"strings\", \"fs\", \"time\")\nfn main() {\n\treturn %s\n}", expr)
	got, err := runCode(script)
	if err != nil {
		t.Errorf("%s: %v", expr, err)
		return
	}
	if got.String() != want {
		t.Errorf("%s = %s, want %s", expr, got.String(), want)
	}
}

func stdFails(t *testing.T, expr, wantInError string) {
	t.Helper()
	script := fmt.Sprintf("package main imports(\"math\", \"strings\", \"fs\", \"time\")\nfn main() {\n\treturn %s\n}", expr)
	_, err := runCode(script)
	if err == nil {
		t.Errorf("%s: expected an error", expr)
		return
	}
	if !strings.Contains(err.Error(), wantInError) {
		t.Errorf("%s: error %q does not mention %q", expr, err.Error(), wantInError)
	}
}

func TestStdMath(t *testing.T) {
	for expr, want := range map[string]string{
		"math.floor(2.7)":         "2",
		"math.ceil(2.1)":          "3",
		"math.round(2.5)":         "3",
		"math.trunc(-2.7)":        "-2",
		"math.abs(-4)":            "4",
		"math.sqrt(16)":           "4",
		"math.pow(2, 10)":         "1024",
		"math.min(3, 1, 2)":       "1",
		"math.max([3, 9, 2])":     "9",
		"math.clamp(15, 0, 10)":   "10",
		"math.sign(-3)":           "-1",
		"math.hypot(3, 4)":        "5",
		"math.pi > 3.14":          "true",
		"math.isNaN(math.nan)":    "true",
		"math.isFinite(math.inf)": "false",
		"math.log2(8)":            "3",
		"math.random() < 1":       "true",
		"math.randomInt(4, 4)":    "4",
	} {
		stdCase(t, expr, want)
	}
	stdFails(t, `math.sqrt("x")`, "number")
	stdFails(t, `math.max(1, "x")`, "position 2")
	stdFails(t, `math.randomInt(5, 1)`, "upper bound")
}

func TestStdStrings(t *testing.T) {
	for expr, want := range map[string]string{
		`strings.split("a,b,c", ",").len()`:       "3",
		`strings.join(["a", "b"], "-")`:           "a-b",
		`strings.fields("  a  b c ").len()`:       "3",
		`strings.lines("x\ny\n").len()`:           "2",
		`strings.contains("hello", "ell")`:        "true",
		`strings.hasPrefix("hello", "he")`:        "true",
		`strings.hasSuffix("hello", "lo")`:        "true",
		`strings.index("héllo", "l")`:             "2",
		`strings.lastIndex("hello", "l")`:         "3",
		`strings.index("hello", "z")`:             "-1",
		`strings.count("banana", "a")`:            "3",
		`strings.replace("a-b-c", "-", "+")`:      "a+b+c",
		`strings.replaceFirst("a-b-c", "-", "+")`: "a+b-c",
		`strings.repeat("ab", 3)`:                 "ababab",
		`strings.reverse("héllo")`:                "olléh",
		`strings.upper("abc")`:                    "ABC",
		`strings.lower("ABC")`:                    "abc",
		`strings.trim("  x  ")`:                   "x",
		`strings.trimStart("  x  ")`:              "x  ",
		`strings.trimEnd("  x  ")`:                "  x",
		`strings.trimPrefix("v1.2", "v")`:         "1.2",
		`strings.trimSuffix("a.ev", ".ev")`:       "a",
		`strings.padStart("7", 3, "0")`:           "007",
		`strings.padEnd("ab", 5, "xy")`:           "abxyx",
		`strings.chars("héy").len()`:              "3",
		`strings.len("héy")`:                      "3",
		`strings.parseNumber(" 2.5 ") * 2`:        "5",
	} {
		stdCase(t, expr, want)
	}
	stdFails(t, `strings.parseNumber("abc")`, "not a number")
	stdFails(t, `strings.repeat("a", -1)`, "negative")
	stdFails(t, `strings.join([1], ",")`, "not a string")
}

func TestStdFs(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	script := fmt.Sprintf(`package main imports("fs")
fn main() {
	root := %q
	sub := fs.join(root, "a/b")
	fs.mkdir(sub)
	file := fs.join(sub, "note.txt")
	fs.writeFile(file, "one")
	fs.appendFile(file, ",two")
	text := fs.readText(file)
	listed := fs.list(sub)
	moved := fs.join(root, "moved.txt")
	fs.rename(file, moved)
	gone := !fs.exists(file)
	fs.remove(fs.join(root, "a"))
	return [text, listed.len(), fs.isDir(sub), gone, fs.exists(moved), fs.base(moved), fs.ext(moved), fs.readFile(moved).len()]
}`, dir)
	got, err := runCode(script)
	if err != nil {
		t.Fatal(err)
	}
	if want := `["one,two", 1, false, true, true, "moved.txt", ".txt", 7]`; got.String() != want {
		t.Fatalf("got %s, want %s", got.String(), want)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatalf("fs.remove left the directory behind: %v", err)
	}
	stdFails(t, `fs.readText("no/such/file")`, "no/such/file")
	stdFails(t, `fs.writeFile("x", 1)`, "string or a buffer")
}

func TestStdTime(t *testing.T) {
	for expr, want := range map[string]string{
		`time.now() > 1700000000000`:         "true",
		`time.since(time.now()) >= 0`:        "true",
		`time.format(0, "2006-01-02")`:       "1970-01-01",
		`time.parse("2026-01-02T03:04:05Z")`: "1767323045000",
		`time.hour / time.minute`:            "60",
	} {
		stdCase(t, expr, want)
	}
	stdFails(t, `time.parse("yesterday")`, "yesterday")

	slept, err := runCode(`package main imports("time")
fn main() {
	start := time.clock()
	time.sleep(5)
	return time.clock() - start >= 4
}`)
	if err != nil || slept.String() != "true" {
		t.Fatalf("time.sleep then time.clock: %v %v", slept, err)
	}
}
