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
	ImportsResolver: StandardLibraryResolver,
}

// StandardLibrary returns all of the standard library package contructors
func StandardLibraryResolver(name string) vm.Package {
	if constructor, exists := StandardLibraryConstructors[name]; exists {
		return constructor()
	}
	panic(fmt.Errorf("constructor not found for '%v'", name))
}

// New creates a new VM instance with the standard library pre-configured.
func New() *vm.Instance {
	return vm.New(Defaults)
}

// Run executes an Evie script using a new default VM instance and returns the result Value.
func Run(script string) (vm.Value, error) {
	return New().EvalString(script)
}

// Eval executes an Evie script and returns the unboxed native Go value.
func Eval(script string) (any, error) {
	val, err := Run(script)
	if err != nil {
		return nil, err
	}
	return val.ToGo(), nil
}
