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
	script := fmt.Sprintf("package main imports(\"math\", \"fs\", \"time\")\nfn main() {\n\treturn %s\n}", expr)
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
	script := fmt.Sprintf("package main imports(\"math\", \"fs\", \"time\")\nfn main() {\n\treturn %s\n}", expr)
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

func TestStringMethods(t *testing.T) {
	for expr, want := range map[string]string{
		`"a,b,c".split(",").len()`:       "3",
		`"  a  b c ".fields().len()`:     "3",
		`"x\ny\n".lines().len()`:         "2",
		`"hello".contains("ell")`:        "true",
		`"hello".startsWith("he")`:       "true",
		`"hello".endsWith("lo")`:         "true",
		`"héllo".index("l")`:             "2",
		`"hello".lastIndex("l")`:         "3",
		`"hello".index("z")`:             "-1",
		`"banana".count("a")`:            "3",
		`"a-b-c".replace("-", "+")`:      "a+b+c",
		`"a-b-c".replaceFirst("-", "+")`: "a+b-c",
		`"ab".repeat(3)`:                 "ababab",
		`"héllo".reverse()`:              "olléh",
		`"abc".toUpper()`:                "ABC",
		`"ABC".toLower()`:                "abc",
		`"  x  ".trim()`:                 "x",
		`"  x  ".trimStart()`:            "x  ",
		`"  x  ".trimEnd()`:              "  x",
		`"v1.2".trimPrefix("v")`:         "1.2",
		`"a.ev".trimSuffix(".ev")`:       "a",
		`"7".padStart(3, "0")`:           "007",
		`"ab".padEnd(5, "xy")`:           "abxyx",
		`"héy".chars().len()`:            "3",
		`"héy".len()`:                    "3",
		`"héllo".slice(1, 4)`:            "éll",
		`"héllo".slice(2)`:               "llo",
		`"héllo".slice(-3, -1)`:          "ll",
		`number(" 2.5 ") * 2`:            "5",
	} {
		stdCase(t, expr, want)
	}
	stdFails(t, `number("abc")`, "cannot convert")
	stdFails(t, `"a".repeat(-1)`, "negative")
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
		`json.decode("{bad")`,
		`json.decode("{\"a\":1,\"a\":2}")`,
		`json.decode("1 2")`,
	} {
		script := "package main imports(\"json\", \"math\")\nfn main() {\n\t" + bad + "\n}"
		if _, err := runCode(script); err == nil {
			t.Errorf("%s: expected json error, got nil", bad)
		}
	}

	// Verify nested decode types (booleans, floats, empty containers, null, escapes)
	val, err = runCode(`package main imports("json")
	fn main() {
		d := json.decode("{\"n\":null,\"f\":false,\"pi\":3.5,\"s\":\"a\\nb\",\"empty\":[],\"obj\":{}}")
		return [d.n == nil, d.f, d.pi * 2, d.s == "a\nb", d.empty.len(), d.obj.len()]
	}`)
	if err != nil || val.String() != "[true, false, 7, true, 0, 0]" {
		t.Fatalf("json.decode types = %v, %v", val, err)
	}
}

func TestStdHardeningGuards(t *testing.T) {
	stdCase(t, `math.isInf(math.inf) && math.isInf(-math.inf) && !math.isInf(1)`, "true")
	stdCase(t, "\"\u00a0hi\".trimStart()", "hi")
	stdCase(t, "\"hi\u00a0\".trimEnd()", "hi")
	stdCase(t, `"\n".lines().len()`, "1")
	stdCase(t, `"".lines().len()`, "0")

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
	stdFails(t, `"a".repeat(100000000)`, "maximum string size")
	stdFails(t, `"a".padStart(100000000, "x")`, "maximum string size")
}

func TestArrayMethods(t *testing.T) {
	for expr, want := range map[string]string{
		`[3, 1, 2].sort()`:                                       "[1, 2, 3]",
		`[2.5, -1, 0].sort()`:                                    "[-1, 0, 2.5]",
		`["pear", "apple", "Fig", "é"].sort()`:                   `["Fig", "apple", "pear", "é"]`,
		`[].sort()`:                                              "[]",
		`[3, 1, 2].sort(fn(a, b) => a > b)`:                      "[3, 2, 1]",
		`["bb", "a", "ccc"].sort(fn(a, b) => a.len() < b.len())`: `["a", "bb", "ccc"]`,
		// stable: equal elements keep their order
		`[{k: 1, n: "a"}, {k: 0, n: "b"}, {k: 1, n: "c"}, {k: 0, n: "d"}].sort(fn(a, b) => a.k < b.k).map(fn(x) => x.n)`: `["b", "d", "a", "c"]`,
		`[1, 2, 3, 4].reduce(fn(s, x) => s + x)`:    "10",
		`[1, 2, 3].reduce(fn(s, x) => s + x, 10)`:   "16",
		`[].reduce(fn(s, x) => s + x, 0)`:           "0",
		`[7].reduce(fn(s, x) => s + x)`:             "7",
		`["a", "b"].reduce(fn(s, x) => s + x, ">")`: ">ab",
		`[5, 6, 7].index(6)`:                        "1",
		`[5, 6, 7].index(8)`:                        "-1",
		`[5, "6", 6].indexOf(6)`:                    "2",
		`[5, 6, 7].find(fn(x) => x > 5)`:            "6",
		`[5, 6, 7].find(fn(x) => x > 9)`:            "nil",
		`[5, 6, 7].findIndex(fn(x) => x > 5)`:       "1",
		`[5, 6, 7].findIndex(fn(x) => x > 9)`:       "-1",
		`[5, 6, 7].some(fn(x) => x == 7)`:           "true",
		`[5, 6, 7].some(fn(x) => x == 8)`:           "false",
		`[].some(fn(x) => true)`:                    "false",
		`[5, 6, 7].every(fn(x) => x > 4)`:           "true",
		`[5, 6, 7].every(fn(x) => x > 5)`:           "false",
		`[].every(fn(x) => false)`:                  "true",
	} {
		stdCase(t, expr, want)
	}
	stdFails(t, `[1, "a"].sort()`, "element 1 is a string but element 0 is a number")
	stdFails(t, `[{}, {}].sort()`, "cannot order an object without a function")
	stdFails(t, `[2, 1].sort(fn(a, b) => a - b)`, "must return a bool")
	stdFails(t, `[2, 1].sort(fn(a, b) => a.missing)`, "no member 'missing'")
	stdFails(t, `[1].sort(1, 2)`, "sort takes 0 or 1 arguments")
	stdFails(t, `[].reduce(fn(s, x) => s)`, "an empty array needs a starting value")
	stdFails(t, `[1].reduce(fn(s, x) => s, 0, 1)`, "reduce takes 1 or 2 arguments")
	stdFails(t, `[1].find(5)`, "must be callable")
}

// sort returns a sorted copy and leaves the array as it was
func TestArraySortCopies(t *testing.T) {
	stdCase(t, `[[3, 1, 2]].map(fn(xs) => [xs.sort(), xs])[0]`, "[[1, 2, 3], [3, 1, 2]]")
}

// < <= > >= order strings by code point, whichever shape the compiler gives
// the operands: constants, locals, other expressions, and conditions
func TestStringOrdering(t *testing.T) {
	stdCase(t, `["a" < "b", "b" < "a", "a" <= "a", "a" >= "b", "B" < "a", "é" > "z", "ab" > "a", "" < "a"]`,
		"[true, false, true, false, true, true, true, true]")
	stdCase(t, `[1].map(fn(_) {
		a := "apple"
		b := "banana"
		var hits := []
		if a < b { hits.push("local < local") }
		if a < "b" { hits.push("local < const") }
		if "z" > b { hits.push("const > local") }
		if a.toUpper() < b { hits.push("expr < local") }
		if a <= a.slice(0) { hits.push("local <= expr") }
		if !(a.slice(0) > b.slice(0)) { hits.push("expr > expr") }
		x := a < b
		var y := false
		y = b >= a
		var i := "a"
		while i < "aaaa" { i += "a" }
		hits.push(x && y && i == "aaaa")
		return hits
	})[0]`, `["local < local", "local < const", "const > local", "expr < local", "local <= expr", "expr > expr", true]`)
	stdFails(t, `1 < "a"`, "cannot apply '<' to a number and a string")
	stdFails(t, `[1].map(fn(_) {
		a := "a"
		return a >= nil
	})`, "cannot apply '>=' to a string and nil")
}
