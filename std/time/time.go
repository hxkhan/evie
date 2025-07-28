package time

import (
	"time"

	"github.com/hxkhan/evie/vm"
)

func Constructor() map[string]*vm.Value {
	timer := vm.BoxGoFunc(timer)

	return map[string]*vm.Value{
		"timer": &timer,
	}
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
