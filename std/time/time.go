package time

import (
	"time"

	"github.com/hxkhan/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage("io")
	pkg.SetSymbol("timer", vm.BoxGoFunc(timer))
	return pkg
}

func timer(duration vm.Value) (vm.Value, *vm.Exception) {
	if duration, ok := duration.AsFloat64(); ok {
		return vm.NewTask(func() (vm.Value, *vm.Exception) {
			time.Sleep(time.Millisecond * time.Duration(duration))
			return vm.Value{}, nil
		}), nil
	}
	return vm.Value{}, vm.ErrTypes
}
