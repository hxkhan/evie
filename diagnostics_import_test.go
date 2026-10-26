package evie_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/vm"
)

// an error inside an imported package shows that package's file, then where it
// was imported from
func TestRenderedErrorInsideImport(t *testing.T) {
	var eng *evie.Engine
	eng = evie.New(evie.Options{Resolve: func(path string) (*vm.Package, error) {
		return eng.Load("geometry.ev", []byte("package geometry\n\nfn area(w, h) {\n    return w * heigth\n}\n"))
	}})
	_, err := eng.Load("main.ev", []byte("package main imports(\"geometry\")\n\nfn main() => geometry.area(2, 3)\n"))
	var r interface{ Diagnostic() *diag.Report }
	if !errors.As(err, &r) {
		t.Fatalf("expected a diagnostic, got %v", err)
	}
	got := r.Diagnostic().Render(diag.Palette{})
	want := `error: cannot find 'heigth' in this scope
 --> geometry.ev:4:16
  |
4 |     return w * heigth
  |                ^^^^^^
  |
 ::: main.ev:1:22
  |
1 | package main imports("geometry")
  |                      ---------- 'geometry' is imported here
  |`
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("rendering differs\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
