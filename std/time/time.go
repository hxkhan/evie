package time

import (
	"time"

	"hxkhan.dev/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage("io")
	pkg.SetSymbol("wait", vm.BoxGoFuncUnsynced(wait))
	return pkg
}

func wait(duration vm.Value) (vm.Value, *vm.Exception) {
	if duration, ok := duration.AsFloat64(); ok {
		time.Sleep(time.Millisecond * time.Duration(duration))
		return vm.Value{}, nil
	}
	return vm.Value{}, vm.ErrTypes
}
