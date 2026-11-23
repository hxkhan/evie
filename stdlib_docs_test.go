package evie_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hxkhan/evie/std/fs"
	"github.com/hxkhan/evie/std/io"
	"github.com/hxkhan/evie/std/json"
	"github.com/hxkhan/evie/std/math"
	"github.com/hxkhan/evie/std/time"
	"github.com/hxkhan/evie/vm"
)

var stdPackages = map[string]*vm.Package{
	"io": io.Construct(), "math": math.Construct(), "fs": fs.Construct(),
	"time": time.Construct(), "json": json.Construct(),
}

// every member of the standard library tells tools how to call it and what it does
func TestStdlibIsDocumented(t *testing.T) {
	for name, pkg := range stdPackages {
		for _, member := range pkg.Members() {
			m, ok := pkg.Describe(member)
			if !ok || m.Doc == "" || m.Signature == "" {
				t.Errorf("%s.%s has no description", name, member)
				continue
			}
			if !strings.HasPrefix(m.Signature, member) {
				t.Errorf("%s.%s: the signature %q does not start with its name", name, member, m.Signature)
			}
		}
	}
}

// the guide's standard library tables name every member, and mark exactly the
// ones that yield
func TestGuideMatchesStdlib(t *testing.T) {
	text, err := os.ReadFile("docs/syntax.md")
	if err != nil {
		t.Fatal(err)
	}
	section := string(text)
	section = section[strings.Index(section, "## 12. Standard Library"):]
	if end := strings.Index(section[3:], "\n## "); end >= 0 {
		section = section[:end+3]
	}

	listed := map[string]map[string]bool{} // package -> members the guide names
	heading := regexp.MustCompile("^### `([a-z]+)`")
	name := regexp.MustCompile("`(?:[a-z]+\\.)?([A-Za-z0-9]+)")
	var pkgName string
	for _, line := range strings.Split(section, "\n") {
		if m := heading.FindStringSubmatch(line); m != nil {
			pkgName = m[1]
			listed[pkgName] = map[string]bool{}
			continue
		}
		pkg := stdPackages[pkgName]
		if pkg == nil || !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(line, "|")
		yields := strings.Contains(line, "**yields**")
		for _, m := range name.FindAllStringSubmatch(cells[1], -1) {
			member := m[1]
			listed[pkgName][member] = true
			d, ok := pkg.Describe(member)
			if !ok {
				t.Errorf("the guide lists %s.%s, which does not exist", pkgName, member)
				continue
			}
			if d.Yields != yields {
				t.Errorf("%s.%s: the guide says yields=%v, the code says %v", pkgName, member, yields, d.Yields)
			}
		}
	}
	for pkgName, pkg := range stdPackages {
		for _, member := range pkg.Members() {
			if !listed[pkgName][member] {
				t.Errorf("the guide does not list %s.%s", pkgName, member)
			}
		}
	}
}
