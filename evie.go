package evie

import (
	"fmt"

	"github.com/hxkhan/evie/std/fs"
	"github.com/hxkhan/evie/std/io"
	"github.com/hxkhan/evie/std/lists"
	"github.com/hxkhan/evie/std/strings"
	"github.com/hxkhan/evie/std/time"
	"github.com/hxkhan/evie/vm"
)

var StandardLibraryConstructors = map[string]func() vm.Package{
	"io":      io.Construct,
	"fs":      fs.Construct,
	"time":    time.Construct,
	"strings": strings.Construct,
	"lists":   lists.Construct,
}

var Defaults = vm.Options{
	LogCache:        false,
	LogCaptures:     false,
	DisableInlining: false,
	Metrics:         false,
	TopLevelLogic:   true,
	ImportResolver:  StandardLibraryResolver,
}

// StandardLibrary returns all of the standard library package contructors
func StandardLibraryResolver(name string) vm.Package {
	if constructor, exists := StandardLibraryConstructors[name]; exists {
		return constructor()
	}
	panic(fmt.Errorf("constructor not found for '%v'", name))
}
