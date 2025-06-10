package time

import (
	"time"

	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/std"
)

func Export() {
	std.ImportFn(timer)
}

func timer(duration core.Value) (core.Value, error) {
	if duration, ok := duration.AsInt64(); ok {
		return core.NewTask(func() (core.Value, error) {
			time.Sleep(time.Millisecond * time.Duration(duration))
			return core.Value{}, nil
		}), nil
	}
	return core.Value{}, core.ErrTypes
}
