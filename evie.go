package evie

import (
	"github.com/hxkhan/evie/std/fs"
	"github.com/hxkhan/evie/std/time"
	"github.com/hxkhan/evie/vm"
)

var Defaults = vm.Options{
	PrintLogs:          false,
	DisableInlining:    false,
	ObserveIt:          false,
	TopLevelLogic:      true,
	PackageContructors: StandardLibrary(),
}

// StandardLibrary returns all of the standard library package contructors
func StandardLibrary() []vm.PackageContructor {
	return []vm.PackageContructor{
		fs.Constructor,
		time.Constructor,
	}
}
