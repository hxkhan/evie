// Package time is Evie's time package. Times are numbers of milliseconds
// since the Unix epoch, so they compare and subtract like any other number.
package time

import (
	"context"
	"time"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the time package
func Construct() *vm.Package {
	pkg := vm.NewPackage("time")
	start := time.Now()

	// wait sleeps without holding up the other tasks, and wakes early when the
	// script is stopped
	wait := pkg.Native("wait", vm.NewFunc("wait", 1, func(c vm.Call) (vm.Value, error) {
		ms, err := c.Number(0)
		if err != nil {
			return vm.Value{}, err
		}
		timer := time.NewTimer(time.Duration(ms * float64(time.Millisecond)))
		defer timer.Stop()
		select {
		case <-timer.C:
			return vm.Value{}, nil
		case <-c.Context().Done():
			return vm.Value{}, context.Cause(c.Context())
		}
	}))
	wait.Blocking = true
	pkg.Set("timer", vm.BoxGoFunc(wait))
	pkg.Set("sleep", vm.BoxGoFunc(wait))

	pkg.Native("now", vm.Func0(func() float64 { return millis(time.Now()) }))
	pkg.Native("since", vm.Func1(func(ms float64) float64 { return millis(time.Now()) - ms }))
	pkg.Native("clock", vm.Func0(func() float64 {
		return float64(time.Since(start).Nanoseconds()) / 1e6
	}))

	pkg.Variadic("format", 1, func(c vm.Call) (vm.Value, error) {
		ms, err := c.Number(0)
		if err != nil {
			return vm.Value{}, err
		}
		layout := time.RFC3339
		if c.Len() > 1 {
			if layout, err = c.String(1); err != nil {
				return vm.Value{}, err
			}
		}
		return vm.BoxString(fromMillis(ms).Format(layout)), nil
	})
	pkg.Native("parse", vm.Func1E(func(s string) (float64, error) {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return 0, err
		}
		return millis(t), nil
	}))

	pkg.Set("second", vm.BoxNumber(1000))
	pkg.Set("minute", vm.BoxNumber(60*1000))
	pkg.Set("hour", vm.BoxNumber(60*60*1000))
	pkg.Set("day", vm.BoxNumber(24*60*60*1000))
	return pkg
}

func millis(t time.Time) float64 { return float64(t.UnixMicro()) / 1000 }

func fromMillis(ms float64) time.Time { return time.UnixMicro(int64(ms * 1000)) }
