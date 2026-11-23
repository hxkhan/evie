package analysis_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hxkhan/evie/analysis"
)

const mainEv = `package main imports("io", "./util", "./lib")

const limit := 3

class Animal(name) {
    fn speak() => this.name
}

class Dog(name) : Animal {
    var tricks := 0
    fn bark(times) {
        for i in [1, 2] { io.println(this.speak()) }
        return util.greet(times) + lib.twice(limit)
    }
}

fn main() {
    d := Dog("rex")
    total := d.bark(limit)
    return total
}
`

func workspace() (fstest.MapFS, *analysis.Workspace) {
	fsys := fstest.MapFS{
		"main.ev":  {Data: []byte(mainEv)},
		"util.ev":  {Data: []byte("package util\nfn greet(n) => \"hi \" + n\n")},
		"lib/a.ev": {Data: []byte("package lib\nfn twice(n) => half(n) * 4\n")},
		"lib/b.ev": {Data: []byte("package lib\nfn half(n) => n / 2\n")},
	}
	return fsys, analysis.NewWorkspace(fsys)
}

// at is the position of the nth (from 0) occurrence of word on a line of text
func at(t *testing.T, text string, line int, word string, nth int) analysis.Position {
	t.Helper()
	l := strings.Split(text, "\n")[line]
	col := -1
	for range nth + 1 {
		i := strings.Index(l[col+1:], word)
		if i < 0 {
			t.Fatalf("%q is not on line %d: %q", word, line, l)
		}
		col += 1 + i
	}
	return analysis.Position{Line: line, Character: col}
}

func TestUpdateFindsNoErrorsInCorrectCode(t *testing.T) {
	_, w := workspace()
	diags := w.Update("main.ev")
	for _, file := range []string{"main.ev", "util.ev", "lib/a.ev", "lib/b.ev"} {
		d, covered := diags[file]
		if !covered {
			t.Errorf("the analysis of main.ev does not cover %s", file)
		}
		if analysis.HasErrors(d) {
			t.Errorf("%s: unexpected errors %+v", file, d)
		}
	}
	// a file of a folder package is checked with the rest of its folder
	if d := w.Update("lib/a.ev"); len(d["lib/a.ev"]) != 0 || len(d["lib/b.ev"]) != 0 {
		t.Errorf("lib/a.ev should check cleanly with lib/b.ev: %+v", d)
	}
}

func TestUpdateReportsErrors(t *testing.T) {
	fsys, w := workspace()

	// every syntax error of a file
	fsys["main.ev"] = &fstest.MapFile{Data: []byte("package main\nfn main() {\n    x := )\n    y := 1 +\n}\n")}
	d := w.Update("main.ev")["main.ev"]
	if len(d) != 2 || d[0].Range.Start.Line != 2 || d[1].Range.Start.Line != 4 {
		t.Fatalf("want errors on lines 3 and 5, got %+v", d)
	}

	// compile errors, with columns counted the way editors count them
	src := "package main\nfn main() {\n    s := \"😀\"; return nope\n}\n"
	fsys["main.ev"] = &fstest.MapFile{Data: []byte(src)}
	d = w.Update("main.ev")["main.ev"]
	if len(d) != 1 || !strings.Contains(d[0].Message, "cannot find 'nope'") {
		t.Fatalf("want one compile error, got %+v", d)
	}
	// the emoji is 4 bytes but 2 UTF-16 units
	if got, want := d[0].Range.Start.Character, strings.Index(src[strings.Index(src, "    s"):], "nope")-2; got != want {
		t.Errorf("the error starts at character %d, want %d", got, want)
	}

	// an error in an imported file is reported in that file
	fsys["main.ev"] = &fstest.MapFile{Data: []byte(mainEv)}
	fsys["util.ev"] = &fstest.MapFile{Data: []byte("package util\nfn greet(n) => missing\n")}
	all := w.Update("main.ev")
	if len(all["util.ev"]) != 1 || len(all["main.ev"]) != 0 {
		t.Fatalf("want the error in util.ev only, got %+v", all)
	}
}

func TestDefinitionAndReferences(t *testing.T) {
	_, w := workspace()
	w.Update("main.ev")
	cases := []struct {
		line int
		word string
		nth  int
		file string
		want int // the line of the declaration
	}{
		{12, "greet", 0, "util.ev", 1},  // a function in another file
		{12, "twice", 0, "lib/a.ev", 1}, // a function in a folder package
		{12, "limit", 0, "main.ev", 2},  // a package constant
		{12, "times", 0, "main.ev", 10}, // a parameter
		{11, "speak", 0, "main.ev", 5},  // an inherited method
		{17, "Dog", 0, "main.ev", 8},    // a class
		{19, "total", 0, "main.ev", 18}, // a local
	}
	for _, c := range cases {
		loc, ok := w.Definition("main.ev", at(t, mainEv, c.line, c.word, c.nth))
		if !ok || loc.Path != c.file || loc.Range.Start.Line != c.want {
			t.Errorf("%s on line %d: got %+v %v, want %s:%d", c.word, c.line, loc, ok, c.file, c.want)
		}
	}
	if _, ok := w.Definition("main.ev", at(t, mainEv, 11, "println", 0)); ok {
		t.Error("a host function has no definition to go to")
	}

	refs := w.References("main.ev", at(t, mainEv, 2, "limit", 0), true)
	if len(refs) != 3 {
		t.Fatalf("limit: want its declaration and two uses, got %+v", refs)
	}
	if refs := w.References("main.ev", at(t, mainEv, 2, "limit", 0), false); len(refs) != 2 {
		t.Errorf("limit without its declaration: got %d", len(refs))
	}
}

func TestHover(t *testing.T) {
	_, w := workspace()
	w.Update("main.ev")
	cases := []struct {
		line int
		word string
		want []string
	}{
		{11, "println", []string{"io.println(...values)", "Writes the values"}},
		{10, "bark", []string{"fn Dog.bark(times)"}},
		{12, "times", []string{"a parameter of fn bark(times)", "parameter"}},
		{12, "greet", []string{"fn greet(n)", "util.ev"}},
		{12, "limit", []string{"const limit := 3", "global"}},
		{0, "io", []string{"package `io`"}},
	}
	for _, c := range cases {
		h, ok := w.Hover("main.ev", at(t, mainEv, c.line, c.word, 0))
		if !ok {
			t.Errorf("%s: no hover", c.word)
			continue
		}
		for _, want := range c.want {
			if !strings.Contains(h.Contents, want) {
				t.Errorf("%s: hover %q lacks %q", c.word, h.Contents, want)
			}
		}
	}
}

func TestSymbols(t *testing.T) {
	_, w := workspace()
	syms := w.Symbols("main.ev")
	var names []string
	for _, s := range syms {
		names = append(names, s.Name)
	}
	if got := strings.Join(names, " "); got != "limit Animal Dog main" {
		t.Fatalf("outline: %q", got)
	}
	var dog []string
	for _, c := range syms[2].Children {
		dog = append(dog, c.Name)
	}
	if got := strings.Join(dog, " "); got != "name tricks bark" {
		t.Errorf("Dog's members: %q", got)
	}
	if syms[2].Range.End.Line != 14 {
		t.Errorf("Dog ends on line %d, want 14", syms[2].Range.End.Line)
	}
}

func TestComplete(t *testing.T) {
	_, w := workspace()
	w.Update("main.ev")
	labels := func(items []analysis.Completion) map[string]analysis.Completion {
		m := map[string]analysis.Completion{}
		for _, c := range items {
			m[c.Label] = c
		}
		return m
	}

	// members of a package, typed into a line that does not parse yet
	text := strings.Replace(mainEv, "    return total\n", "    io.\n    return total\n", 1)
	items := labels(w.Complete("main.ev", analysis.Position{Line: 19, Character: 7}, text))
	if _, ok := items["println"]; !ok {
		t.Errorf("io. should offer println, got %v", items)
	}
	if !strings.Contains(items["readln"].Documentation, "yields") {
		t.Errorf("io.readln should say it yields: %+v", items["readln"])
	}

	// members of this, the parent's included
	text = strings.Replace(mainEv, "        return util", "        this.\n        return util", 1)
	items = labels(w.Complete("main.ev", analysis.Position{Line: 12, Character: 13}, text))
	for _, want := range []string{"name", "tricks", "bark", "speak"} {
		if _, ok := items[want]; !ok {
			t.Errorf("this. inside Dog should offer %s, got %v", want, items)
		}
	}

	// names in scope inside main: its locals and the package's names, not
	// another function's parameters or loop variables
	items = labels(w.Complete("main.ev", at(t, mainEv, 19, "total", 0), mainEv))
	for _, want := range []string{"d", "total", "limit", "Dog", "util", "main", "return", "string"} {
		if _, ok := items[want]; !ok {
			t.Errorf("inside main, %s should be offered", want)
		}
	}
	for _, unwanted := range []string{"times", "i", "tricks"} {
		if _, ok := items[unwanted]; ok {
			t.Errorf("inside main, %s should not be offered", unwanted)
		}
	}
}

// while a file does not parse, names come from the last version that did
func TestNamesSurviveABrokenEdit(t *testing.T) {
	fsys, w := workspace()
	w.Update("main.ev")
	fsys["main.ev"] = &fstest.MapFile{Data: []byte(strings.Replace(mainEv, "fn main() {", "fn main() { x := )", 1))}
	if d := w.Update("main.ev")["main.ev"]; len(d) == 0 {
		t.Fatal("the broken file should have an error")
	}
	loc, ok := w.Definition("main.ev", at(t, mainEv, 12, "greet", 0))
	if !ok || loc.Path != "util.ev" {
		t.Errorf("go to definition should still work: %+v %v", loc, ok)
	}
}

// main files that each declare fn main are programs of their own; main files
// of which one declares it are one program split over files
func TestMainFilesOfAFolder(t *testing.T) {
	separate := fstest.MapFS{
		"a.ev": {Data: []byte("package main\nfn helper() => 1\nfn main() => helper()\n")},
		"b.ev": {Data: []byte("package main\nfn helper() => 2\nfn main() => helper()\n")},
	}
	d := analysis.NewWorkspace(separate).Update("a.ev")
	if len(d["a.ev"]) != 0 {
		t.Errorf("a.ev is a program of its own: %+v", d)
	}
	if _, covered := d["b.ev"]; covered {
		t.Error("checking a.ev should not take in b.ev")
	}

	split := fstest.MapFS{
		"a.ev": {Data: []byte("package main\nfn main() => helper()\n")},
		"b.ev": {Data: []byte("package main\nfn helper() => 2\n")},
	}
	d = analysis.NewWorkspace(split).Update("a.ev")
	if len(d["a.ev"]) != 0 || len(d["b.ev"]) != 0 {
		t.Errorf("a.ev and b.ev are one program: %+v", d)
	}
}

func TestFolding(t *testing.T) {
	_, w := workspace()
	got := map[int]int{}
	for _, r := range w.Folding("main.ev") {
		got[r.StartLine] = r.EndLine
	}
	// class Animal, class Dog, bark's body, main's body; the closing line stays visible
	for start, end := range map[int]int{4: 5, 8: 13, 10: 12, 16: 19} {
		if got[start] != end {
			t.Errorf("want a fold from line %d to %d, got %v", start, end, got)
		}
	}
	if _, ok := got[11]; ok {
		t.Error("a block on one line does not fold")
	}

	fsys := fstest.MapFS{"s.ev": {Data: []byte("x := {\n    a: 1,\n    b: [\n        1,\n        2\n    ]\n}\nswitch x {\ncase 1:\n    y := 2\n}\n")}}
	got = map[int]int{}
	for _, r := range analysis.NewWorkspace(fsys).Folding("s.ev") {
		got[r.StartLine] = r.EndLine
	}
	for start, end := range map[int]int{0: 5, 2: 4, 7: 9} {
		if got[start] != end {
			t.Errorf("object, array and switch: want a fold from line %d to %d, got %v", start, end, got)
		}
	}
}

func TestHighlights(t *testing.T) {
	_, w := workspace()
	w.Update("main.ev")
	marks := w.Highlights("main.ev", at(t, mainEv, 12, "limit", 0))
	if len(marks) != 3 {
		t.Fatalf("want limit's declaration and its two uses, got %+v", marks)
	}
	writes := 0
	for _, m := range marks {
		if m.Kind == 3 {
			writes++
			if m.Range.Start.Line != 2 {
				t.Errorf("the write is the declaration on line 2, got %+v", m)
			}
		}
	}
	if writes != 1 {
		t.Errorf("want one write, got %d", writes)
	}
}

func TestSignatureHelp(t *testing.T) {
	_, w := workspace()
	w.Update("main.ev")
	cases := []struct {
		insert string // replaces "return total" in main, cursor at its end
		label  string
		active int
	}{
		{"util.greet(", "util.greet(n)", 0},
		{"lib.twice(", "lib.twice(n)", 0},
		{"io.println(1, 2, ", "io.println(...values)", 0},
		{"Dog(", "Dog(name)", 0},
		{"util.greet(lib.twice(", "lib.twice(n)", 0},
		{"util.greet(lib.twice(1), ", "util.greet(n)", 1},
		{"helper(1, ", "helper(a, b)", 1},
		{"util.greet(1) + 2", "", 0},
		{"if (", "", 0},
	}
	for _, c := range cases {
		text := strings.Replace(mainEv, "    return total\n", "    helper := fn(a, b) => a + b\n    "+c.insert+"\n", 1)
		pos := analysis.Position{Line: 20, Character: 4 + len(c.insert)}
		// the helper is declared in the text being typed, so give the analysis a version that has it
		if strings.HasPrefix(c.insert, "helper") {
			fsys, w2 := workspace()
			fsys["main.ev"] = &fstest.MapFile{Data: []byte(strings.Replace(mainEv, "    return total\n", "    helper := fn(a, b) => a + b\n    return total\n", 1))}
			w2.Update("main.ev")
			w = w2
		}
		h, ok := w.SignatureHelp("main.ev", pos, text)
		if c.label == "" {
			if ok {
				t.Errorf("%q is in no call, got %+v", c.insert, h)
			}
			continue
		}
		if !ok || h.Signatures[0].Label != c.label || h.ActiveParameter != c.active {
			t.Errorf("%q: got %+v %v, want %s with parameter %d", c.insert, h, ok, c.label, c.active)
		}
	}

	// a method of the class, inherited
	text := strings.Replace(mainEv, "        return util", "        this.speak(\n        return util", 1)
	h, ok := w.SignatureHelp("main.ev", analysis.Position{Line: 12, Character: 19}, text)
	if !ok || h.Signatures[0].Label != "speak()" {
		t.Errorf("this.speak(: got %+v %v", h, ok)
	}

	// a standard function says what it does
	text = strings.Replace(mainEv, "    return total\n", "    io.readln(\n", 1)
	h, ok = w.SignatureHelp("main.ev", analysis.Position{Line: 19, Character: 14}, text)
	if !ok || !strings.Contains(h.Signatures[0].Documentation, "yields") {
		t.Errorf("io.readln(: got %+v %v", h, ok)
	}
}

func TestRename(t *testing.T) {
	edited := func(t *testing.T, fsys fstest.MapFS, edits map[string][]analysis.TextEdit) map[string]string {
		t.Helper()
		out := map[string]string{}
		for file, list := range edits {
			lines := strings.Split(string(fsys[file].Data), "\n")
			// apply from the end, so earlier ranges stay put
			sorted := slices.Clone(list)
			slices.SortFunc(sorted, func(a, b analysis.TextEdit) int {
				if a.Range.Start.Line != b.Range.Start.Line {
					return b.Range.Start.Line - a.Range.Start.Line
				}
				return b.Range.Start.Character - a.Range.Start.Character
			})
			for _, e := range sorted {
				l := lines[e.Range.Start.Line]
				lines[e.Range.Start.Line] = l[:e.Range.Start.Character] + e.NewText + l[e.Range.End.Character:]
			}
			out[file] = strings.Join(lines, "\n")
		}
		return out
	}

	// a constant, everywhere it is used
	fsys, w := workspace()
	w.Update("main.ev")
	edits, err := w.Rename("main.ev", at(t, mainEv, 12, "limit", 0), "cap")
	if err != nil {
		t.Fatal(err)
	}
	out := edited(t, fsys, edits)
	if strings.Count(out["main.ev"], "cap") != 3 || strings.Contains(out["main.ev"], "limit") {
		t.Errorf("limit -> cap:\n%s", out["main.ev"])
	}

	// a function of another package, from where it is used
	edits, err = w.Rename("main.ev", at(t, mainEv, 12, "greet", 0), "welcome")
	if err != nil {
		t.Fatal(err)
	}
	out = edited(t, fsys, edits)
	if !strings.Contains(out["util.ev"], "fn welcome(n)") || !strings.Contains(out["main.ev"], "util.welcome(times)") {
		t.Errorf("greet -> welcome: %v", out)
	}

	// a function of a folder package, used by its other file
	edits, err = w.Rename("lib/b.ev", analysis.Position{Line: 1, Character: 3}, "halve")
	if err != nil {
		t.Fatal(err)
	}
	out = edited(t, fsys, edits)
	if !strings.Contains(out["lib/b.ev"], "fn halve(n)") || !strings.Contains(out["lib/a.ev"], "halve(n) * 4") {
		t.Errorf("half -> halve: %v", out)
	}

	// an import without an alias gets one
	edits, err = w.Rename("main.ev", at(t, mainEv, 11, "io", 0), "out")
	if err != nil {
		t.Fatal(err)
	}
	out = edited(t, fsys, edits)
	if !strings.Contains(out["main.ev"], `imports("io" as out,`) || !strings.Contains(out["main.ev"], "out.println(") {
		t.Errorf("io -> out:\n%s", out["main.ev"])
	}

	// renames that would change what the code means, or cannot be complete
	refused := []struct {
		line int
		word string
		to   string
		why  string
	}{
		{12, "times", "limit", "change what other code refers to"}, // would hide the constant limit
		{19, "total", "d", "break"},                                // declared twice
		{2, "limit", "while", "keyword"},
		{2, "limit", "2x", "not a valid name"},
		{5, "name", "label", "field"},
	}
	for _, c := range refused {
		_, err := w.Rename("main.ev", at(t, mainEv, c.line, c.word, 0), c.to)
		if err == nil || !strings.Contains(err.Error(), c.why) {
			t.Errorf("%s -> %s: got %v, want a refusal mentioning %q", c.word, c.to, err, c.why)
		}
	}
	if _, _, err := w.PrepareRename("main.ev", at(t, mainEv, 0, `"io"`, 0)); err == nil {
		t.Error("the quoted path of an import cannot be renamed")
	}

	// code with errors is not renamed
	fsys["main.ev"] = &fstest.MapFile{Data: []byte(strings.Replace(mainEv, "return total", "return total + nope", 1))}
	w.Update("main.ev")
	if _, err := w.Rename("main.ev", at(t, mainEv, 2, "limit", 0), "cap"); err == nil || !strings.Contains(err.Error(), "fix the errors") {
		t.Errorf("want a refusal while there are errors, got %v", err)
	}
}

func TestHints(t *testing.T) {
	_, w := workspace()
	var hints []string
	for _, d := range w.Update("main.ev")["main.ev"] {
		if d.Severity == analysis.SeverityHint {
			hints = append(hints, d.Message)
		}
	}
	// the loop variable i is the fixture's only unused name
	if len(hints) != 1 || hints[0] != "'i' is declared but never used" {
		t.Errorf("want one hint about i, got %q", hints)
	}

	fsys := fstest.MapFS{
		"main.ev": {Data: []byte("package main imports(\"io\", \"time\")\n\nfn main() {\n    a := 1\n    _b := 2\n    c := 3\n    return a\n}\n")},
		"s.ev":    {Data: []byte("value := 1\necho valeu\n")},
		"m.ev":    {Data: []byte("package main imports(\"io\")\nfn main() { io.pritnln(1) }\n")},
	}
	w = analysis.NewWorkspace(fsys)
	hints = nil
	for _, d := range w.Update("main.ev")["main.ev"] {
		hints = append(hints, d.Message)
	}
	want := []string{"'io' is imported but never used", "'time' is imported but never used", "'c' is declared but never used"}
	if !slices.Equal(hints, want) {
		t.Errorf("got hints %q, want %q", hints, want)
	}

	// a name that is not found, with a similar one, has a quick fix
	for file, fix := range map[string]string{"s.ev": "value", "m.ev": "println"} {
		d := w.Update(file)[file]
		if len(d) != 1 || d[0].Data == nil || d[0].Data.Fixes[0].Edit.NewText != fix {
			t.Errorf("%s: want a fix to %q, got %+v", file, fix, d)
			continue
		}
		r := d[0].Data.Fixes[0].Edit.Range
		line := strings.Split(string(fsys[file].Data), "\n")[r.Start.Line]
		if got := line[r.Start.Character:r.End.Character]; got == fix || len(got) != len(fix) {
			t.Errorf("%s: the fix replaces %q", file, got)
		}
	}
}

func TestWorkspaceSymbols(t *testing.T) {
	_, w := workspace()
	var names []string
	for _, s := range w.WorkspaceSymbols("") {
		names = append(names, s.ContainerName+"."+s.Name+"@"+s.Location.Path)
	}
	want := []string{".twice@lib/a.ev", ".half@lib/b.ev", ".limit@main.ev", ".Animal@main.ev", "Animal.name@main.ev", "Animal.speak@main.ev",
		".Dog@main.ev", "Dog.name@main.ev", "Dog.tricks@main.ev", "Dog.bark@main.ev", ".main@main.ev", ".greet@util.ev"}
	if !slices.Equal(names, want) {
		t.Errorf("got %q, want %q", names, want)
	}
	if got := w.WorkspaceSymbols("brk"); len(got) != 1 || got[0].Name != "bark" || got[0].Location.Range.Start.Line != 10 {
		t.Errorf("searching brk: %+v", got)
	}
}

func TestLinks(t *testing.T) {
	_, w := workspace()
	links := w.Links("main.ev")
	if len(links) != 2 || links[0].Target != "util.ev" || links[1].Target != "lib/a.ev" {
		t.Fatalf("want links to util.ev and lib/a.ev, got %+v", links)
	}
	// the link covers the quoted path
	line := strings.Split(mainEv, "\n")[0]
	if got := line[links[0].Range.Start.Character:links[0].Range.End.Character]; got != `"./util"` {
		t.Errorf("the link covers %q", got)
	}
}

func TestRunnable(t *testing.T) {
	_, w := workspace()
	r, ok := w.Runnable("main.ev")
	if !ok || r.Target != "." || !r.Project || r.Range.Start.Line != 16 {
		t.Errorf("main.ev runs its folder from fn main on line 17, got %+v %v", r, ok)
	}
	if _, ok := w.Runnable("util.ev"); ok {
		t.Error("a package other than main is not a program")
	}
	fsys := fstest.MapFS{
		"ex/a.ev":   {Data: []byte("package main\nfn main() {}\n")},
		"ex/b.ev":   {Data: []byte("package main\nfn main() {}\n")},
		"script.ev": {Data: []byte("echo 1\n")},
	}
	w = analysis.NewWorkspace(fsys)
	if r, ok := w.Runnable("ex/a.ev"); !ok || r.Target != "ex/a.ev" || r.Project {
		t.Errorf("a folder of separate programs runs the file, got %+v", r)
	}
	if r, ok := w.Runnable("script.ev"); !ok || r.Target != "script.ev" || r.Range.Start.Line != 0 {
		t.Errorf("a script runs from its start, got %+v", r)
	}
}
