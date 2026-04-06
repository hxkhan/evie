package time

import (
	"time"

	"github.com/hxkhan/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage()
	pkg.SetSymbol("wait", wait)
	return pkg
}

var wait = vm.BoxGoFunc(&vm.GoFunc{
	Name:      "wait",
	Arguments: 1,
	IsMethod:  false,
	Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		if duration, ok := fbr.GetLocal(0).AsFloat64(); ok {
			time.Sleep(time.Millisecond * time.Duration(duration))
			return vm.Value{}, nil
		}

		return vm.Value{}, vm.ErrTypes
	},
})
