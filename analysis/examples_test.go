package analysis_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hxkhan/evie/analysis"
)

// every example is a correct program: the analysis finds no errors in it, and
// asking about any place in it neither fails nor panics
func TestExamples(t *testing.T) {
	root, err := filepath.Abs("../examples")
	if err != nil {
		t.Fatal(err)
	}
	fsys := os.DirFS(root)
	w := analysis.NewWorkspace(fsys)
	files, _ := filepath.Glob(filepath.Join(root, "*.ev"))
	if len(files) == 0 {
		t.Fatal("no examples found")
	}
	for _, full := range files {
		name := filepath.Base(full)
		if name == "test.ev" {
			continue // the author's scratch file, not an example
		}
		for file, diags := range w.Update(name) {
			for _, d := range diags {
				t.Errorf("%s: %s:%d: %s", name, file, d.Range.Start.Line+1, d.Message)
			}
		}
		text, _ := os.ReadFile(full)
		lines := strings.Split(string(text), "\n")
		named := 0
		for line, l := range lines {
			for col := 0; col <= len(l); col++ {
				pos := analysis.Position{Line: line, Character: col}
				if _, ok := w.Definition(name, pos); ok {
					named++
				}
				w.Hover(name, pos)
				if col%29 == 0 {
					w.References(name, pos, true)
					w.Complete(name, pos, string(text))
				}
			}
		}
		if named == 0 && len(lines) > 5 {
			t.Errorf("%s: no name had a definition", name)
		}
		w.Symbols(name)
	}
}

// how long checking a file takes, which happens as typing pauses
func BenchmarkUpdate(b *testing.B) {
	root, _ := filepath.Abs("../examples")
	w := analysis.NewWorkspace(os.DirFS(root))
	for _, name := range []string{"class.ev", "fib.ev"} {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				w.Update(name)
			}
		})
	}
}
