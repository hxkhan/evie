package time

import (
	"time"

	"github.com/hxkhan/evie/std"
	"github.com/hxkhan/evie/vm"
)

func Export() {
	std.ImportFn(timer)
}

func timer(duration vm.Value) (vm.Value, error) {
	if duration, ok := duration.AsFloat64(); ok {
		return vm.NewTask(func() (vm.Value, error) {
			time.Sleep(time.Millisecond * time.Duration(duration))
			return vm.Value{}, nil
		}), nil
	}
	return vm.Value{}, vm.ErrTypes
}
