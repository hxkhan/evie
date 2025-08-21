package std

import (
	"fmt"

	"hxkhan.dev/evie/std/fs"
	"hxkhan.dev/evie/std/io"
	"hxkhan.dev/evie/std/lists"
	"hxkhan.dev/evie/std/strings"
	"hxkhan.dev/evie/std/time"
	"hxkhan.dev/evie/vm"
)

var Constructors = map[string]func() vm.Package{
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
	ImportsResolver: Resolver,
}

// Resolver returns all of the standard library package contructors
func Resolver(name string) vm.Package {
	if constructor, exists := Constructors[name]; exists {
		return constructor()
	}
	panic(fmt.Errorf("constructor not found for '%v'", name))
}
