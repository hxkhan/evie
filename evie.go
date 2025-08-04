package evie

import (
	"fmt"

	"github.com/hxkhan/evie/std/builtin"
	"github.com/hxkhan/evie/std/fs"
	"github.com/hxkhan/evie/std/io"
	"github.com/hxkhan/evie/std/time"
	"github.com/hxkhan/evie/vm"
)

var StandardLibraryPackageConstructors = map[string]func() vm.Package{
	"io":      io.Construct,
	"fs":      fs.Construct,
	"time":    time.Construct,
	"builtin": builtin.Construct,
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
	if constructor, exists := StandardLibraryPackageConstructors[name]; exists {
		return constructor()
	}
	panic(fmt.Errorf("constructor not found for '%v'", name))
}
