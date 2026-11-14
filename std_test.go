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
		`strings.slice("héllo", 1, 4)`:            "éll",
		`strings.slice("héllo", 2)`:               "llo",
		`strings.slice("héllo", -3, -1)`:          "ll",
		`strings.parseNumber(" 2.5 ") * 2`:        "5",
	} {
		stdCase(t, expr, want)
	}
	stdFails(t, `strings.parseNumber("abc")`, "not a number")
	stdFails(t, `strings.repeat("a", -1)`, "negative")
}

func TestStdFs(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	script := fmt.Sprintf(`package main imports("fs")
fn main() {
	root := %q
	sub := fs.join(root, "a", "b")
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
	return [text, listed.len(), fs.isDir(sub), gone, fs.exists(moved), fs.base(moved), fs.ext(moved), fs.readFile(moved).len(), fs.join("x", "y", "z", "w")]
}`, dir)
	got, err := runCode(script)
	if err != nil {
		t.Fatal(err)
	}
	if want := `["one,two", 1, false, true, true, "moved.txt", ".txt", 7, "x/y/z/w"]`; got.String() != want {
		t.Fatalf("got %s, want %s", got.String(), want)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatalf("fs.remove left the directory behind: %v", err)
	}
	stdFails(t, `fs.readText("no/such/file")`, "no/such/file")
	stdFails(t, `fs.writeFile("x", 1)`, "string or a buffer")
	stdFails(t, `fs.join()`, "at least 1 argument")
}

func TestStdIOInput(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		r.Close()
	}()

	if _, err := w.WriteString("John Doe\r\n25\nthird line with spaces\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()

	got, err := runCode(`package main imports("io")
fn main() {
	name := io.prompt("")
	age := io.prompt("")
	extra := io.readln()
	return [name, age, extra]
}`)
	if err != nil {
		t.Fatal(err)
	}
	want := `["John Doe", "25", "third line with spaces"]`
	if got.String() != want {
		t.Fatalf("got %s, want %s", got.String(), want)
	}
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

	stdCase(t, `time.parse("2026-01-02", "2006-01-02")`, "1767312000000")
	stdFails(t, `time.sleep("x")`, "sleep: argument 1 must be a number")
	stdFails(t, `time.wait(math.nan)`, "NaN")
	stdFails(t, `time.format(0, "2006", "extra")`, "1 or 2 arguments")
	stdFails(t, `time.format(math.nan)`, "finite")
}

func TestStdJSON(t *testing.T) {
	code := `package main imports("json")
	class Point(x, y) {}
	fn main() {
		raw := json.encode({ a: [1, true, nil, "hi"], p: Point(3, 4) })
		decoded := json.decode(raw)
		return [raw, decoded.p.x, decoded.p.y, decoded.p.has("__class")]
	}`
	val, err := runCode(code)
	if err != nil {
		t.Fatalf("json round-trip failed: %v", err)
	}
	want := `["{"a":[1,true,null,"hi"],"p":{"x":3,"y":4}}", 3, 4, false]`
	if val.String() != want {
		t.Fatalf("got %s, want %s", val.String(), want)
	}

	for _, bad := range []string{
		`json.encode(fn() => 1)`,
		`json.encode(task())`,
		`json.encode(json)`,
		`json.encode(error("e"))`,
		`json.encode(buffer("hi"))`,
		`json.encode(math.nan)`,
		`json.encode(math.inf)`,
		`a := []; a.push(a); json.encode(a)`,
		`o := {}; o.self = o; json.encode(o)`,
	} {
		script := "package main imports(\"json\", \"math\")\nfn main() {\n\t" + bad + "\n}"
		if _, err := runCode(script); err == nil {
			t.Errorf("%s: expected json.encode error, got nil", bad)
		}
	}
}

func TestStdHardeningGuards(t *testing.T) {
	stdCase(t, `math.isInf(math.inf) && math.isInf(-math.inf) && !math.isInf(1)`, "true")
	stdCase(t, "strings.trimStart(\"\u00a0hi\")", "hi")
	stdCase(t, "strings.trimEnd(\"hi\u00a0\")", "hi")
	stdCase(t, `strings.lines("\n").len()`, "1")
	stdCase(t, `strings.lines("").len()`, "0")

	// Wide randomInt must not overflow/panic
	val, err := runCode(`package main imports("math")
	fn main() {
		n := math.randomInt(-9000000000000000000, 9000000000000000000)
		return n is number
	}`)
	if err != nil || val.String() != "true" {
		t.Fatalf("wide randomInt failed: %v, %v", val, err)
	}

	stdFails(t, `math.clamp(5, 10, 0)`, "lower bound exceeds")
	stdFails(t, `strings.repeat("a", 100000000)`, "maximum string size")
	stdFails(t, `strings.padStart("a", 100000000, "x")`, "maximum string size")
}
