package time

import (
	"time"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the time package
func Construct() *vm.Package {
	pkg := vm.NewPackage("time")
	wait := pkg.Func("wait", 1, func(c vm.Call) (vm.Value, error) {
		ms, err := c.Number(0)
		if err != nil {
			return vm.Value{}, err
		}
		time.Sleep(time.Duration(ms * float64(time.Millisecond)))
		return vm.Value{}, nil
	})
	wait.Blocking = true
	pkg.Set("timer", vm.BoxGoFunc(wait))
	return pkg
}
