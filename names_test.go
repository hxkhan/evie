package evie_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

// the compiler tells a tool where every name it resolves is declared, across
// functions, closures, classes and packages, without running anything
func TestOnNameResolvesDeclarations(t *testing.T) {
	files := map[string]string{
		"main.ev": `package main imports("io", "./util" as u)

const limit := 3

class Animal(name) {
    fn speak() => this.name
}
class Dog(name) : Animal {
    fn bark(times) {
        var out := ""
        for i in [1, 2] { out += this.speak() }
        return out + u.greet(times) + limit
    }
}
fn main() {
    d := Dog("rex")
    io.println(d.bark(limit))
    helper := fn(x) => x + 1
    return helper(2)
}`,
		"util.ev": "package util\nfn greet(n) => \"hi \" + n",
	}
	fsys := fstest.MapFS{}
	for name, src := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(src)}
	}
	b, err := evie.ParseFile(fsys, "main.ev")
	if err != nil {
		t.Fatal(err)
	}
	var events []vm.NameEvent
	eng := evie.New(evie.Options{FS: fsys, OnName: func(e vm.NameEvent) { events = append(events, e) }})
	defer eng.Close()
	if err := eng.Check(b); err != nil {
		t.Fatal(err)
	}

	lines := map[string][]string{}
	for name, src := range files {
		lines[name] = strings.Split(src, "\n")
	}
	// textAt is what a position covers
	textAt := func(file string, line, col, length int) string {
		text := lines[file][line-1]
		return text[col-1 : col-1+max(length, 1)]
	}

	// use finds the use of name on a line of main.ev and checks what it resolved to
	use := func(line int, name string, kind vm.NameKind, declFile string, declLine int) {
		t.Helper()
		for _, e := range events {
			if e.Declares || e.Name != name || e.Source == nil || e.Source.Name != "main.ev" || e.Pos.Line != line {
				continue
			}
			if got := textAt("main.ev", e.Pos.Line, e.Pos.Column, e.Pos.Length); got != name {
				t.Errorf("line %d: the use of %s covers %q", line, name, got)
			}
			if e.Kind != kind {
				t.Errorf("line %d: %s is a %v, want a %v", line, name, e.Kind, kind)
			}
			if declFile == "" {
				if e.DeclPos.Known() {
					t.Errorf("line %d: %s should have no declaration, got line %d", line, name, e.DeclPos.Line)
				}
				return
			}
			if e.DeclSource == nil || e.DeclSource.Name != declFile || e.DeclPos.Line != declLine {
				file := "<none>"
				if e.DeclSource != nil {
					file = e.DeclSource.Name
				}
				t.Errorf("line %d: %s is declared at %s:%d, want %s:%d", line, name, file, e.DeclPos.Line, declFile, declLine)
				return
			}
			if got := textAt(declFile, e.DeclPos.Line, e.DeclPos.Column, e.DeclPos.Length); !strings.Contains(got, name) {
				t.Errorf("line %d: the declaration of %s covers %q", line, name, got)
			}
			return
		}
		t.Errorf("line %d: no use of %s was reported", line, name)
	}

	use(6, "name", vm.NameField, "main.ev", 5)       // this.name, a constructor field
	use(8, "Animal", vm.NameClass, "main.ev", 5)     // the parent class
	use(11, "out", vm.NameVariable, "main.ev", 10)   // a local
	use(11, "speak", vm.NameField, "main.ev", 6)     // an inherited method
	use(12, "times", vm.NameParameter, "main.ev", 9) // a parameter
	use(12, "u", vm.NameImport, "main.ev", 1)        // an aliased import
	use(12, "greet", vm.NameMember, "util.ev", 2)    // a member of another file's package
	use(12, "limit", vm.NameGlobal, "main.ev", 3)    // a package constant
	use(16, "Dog", vm.NameClass, "main.ev", 8)
	use(17, "io", vm.NameImport, "main.ev", 1)
	use(17, "println", vm.NameMember, "", 0) // a host function has no source
	use(17, "d", vm.NameVariable, "main.ev", 16)
	use(18, "x", vm.NameParameter, "main.ev", 18)
	use(19, "helper", vm.NameVariable, "main.ev", 18)

	// declarations are reported too
	declared := map[string]vm.NameKind{}
	for _, e := range events {
		if e.Declares && e.Source.Name == "main.ev" {
			declared[e.Name] = e.Kind
		}
	}
	for name, kind := range map[string]vm.NameKind{
		"limit": vm.NameGlobal, "Animal": vm.NameClass, "speak": vm.NameField, "bark": vm.NameField,
		"times": vm.NameParameter, "i": vm.NameVariable, "main": vm.NameFunction, "u": vm.NameImport,
	} {
		if declared[name] != kind {
			t.Errorf("the declaration of %s is a %v, want a %v", name, declared[name], kind)
		}
	}
}

// recording names changes nothing about what the compiled code does
func TestOnNameDoesNotChangeResults(t *testing.T) {
	src := "package main\nclass C(n) { fn get() => this.n * 2 }\nfn main() { c := C(21)\n return c.get() }"
	b, err := evie.ParseSource("main.ev", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	eng := evie.New(evie.Options{OnName: func(vm.NameEvent) { count++ }})
	defer eng.Close()
	v, err := eng.RunBundle(t.Context(), b)
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no names were reported")
	}
	if v.String() != "42" {
		t.Fatalf("got %v with names recorded", v)
	}
}
