// Package time is Evie's time package. Times are numbers of milliseconds
// since the Unix epoch, so they compare and subtract like any other number.
package time

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the time package
func Construct() *vm.Package {
	pkg := vm.NewPackage("time")
	start := time.Now()

	// wait sleeps without holding up the other tasks, and wakes early when the
	// script is stopped
	sleepFn := vm.NewFunc("wait", 1, func(c vm.Call) (vm.Value, error) {
		ms, err := c.Number(0)
		if err != nil {
			return vm.Value{}, err
		}
		if math.IsNaN(ms) {
			return vm.Value{}, errors.New("duration must not be NaN")
		}
		if ms <= 0 {
			if err := c.Context().Err(); err != nil {
				return vm.Value{}, context.Cause(c.Context())
			}
			return vm.Value{}, nil
		}
		if math.IsInf(ms, 1) || ms*float64(time.Millisecond) >= float64(math.MaxInt64) {
			<-c.Context().Done()
			return vm.Value{}, context.Cause(c.Context())
		}
		timer := time.NewTimer(time.Duration(ms * float64(time.Millisecond)))
		defer timer.Stop()
		select {
		case <-timer.C:
			return vm.Value{}, nil
		case <-c.Context().Done():
			return vm.Value{}, context.Cause(c.Context())
		}
	})
	sleepFn.Blocking = true
	pkg.Native("wait", sleepFn)
	pkg.Native("sleep", sleepFn)
	pkg.Native("timer", sleepFn)

	pkg.Native("now", vm.Func0(func() float64 { return millis(time.Now()) }))
	pkg.Native("since", vm.Func1(func(ms float64) float64 { return millis(time.Now()) - ms }))
	pkg.Native("clock", vm.Func0(func() float64 {
		return float64(time.Since(start).Nanoseconds()) / 1e6
	}))

	pkg.Variadic("format", 1, func(c vm.Call) (vm.Value, error) {
		if c.Len() > 2 {
			return vm.Value{}, fmt.Errorf("format takes 1 or 2 arguments, got %d", c.Len())
		}
		ms, err := c.Number(0)
		if err != nil {
			return vm.Value{}, err
		}
		t, err := fromMillis(ms)
		if err != nil {
			return vm.Value{}, err
		}
		layout := time.RFC3339
		if c.Len() == 2 {
			if layout, err = c.String(1); err != nil {
				return vm.Value{}, err
			}
		}
		return vm.BoxString(t.UTC().Format(layout)), nil
	})
	pkg.Variadic("parse", 1, func(c vm.Call) (vm.Value, error) {
		if c.Len() > 2 {
			return vm.Value{}, fmt.Errorf("parse takes 1 or 2 arguments, got %d", c.Len())
		}
		s, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		layout := time.RFC3339
		if c.Len() == 2 {
			if layout, err = c.String(1); err != nil {
				return vm.Value{}, err
			}
		}
		t, err := time.Parse(layout, s)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxNumber(millis(t)), nil
	})

	pkg.Set("second", vm.BoxNumber(1000))
	pkg.Set("minute", vm.BoxNumber(60*1000))
	pkg.Set("hour", vm.BoxNumber(60*60*1000))
	pkg.Set("day", vm.BoxNumber(24*60*60*1000))
	// what tools such as editors show for each member
	pkg.Document("now", "now()", "The current time, in milliseconds since the Unix epoch.")
	pkg.Document("since", "since(t)", "Milliseconds elapsed since the time t.")
	pkg.Document("clock", "clock()", "A monotonic clock in milliseconds, for measuring.")
	pkg.Document("wait", "wait(ms)", "Pauses the current task for ms milliseconds.")
	pkg.Document("sleep", "sleep(ms)", "Pauses the current task for ms milliseconds; the same as wait.")
	pkg.Document("timer", "timer(ms)", "Pauses the current task for ms milliseconds; the same as wait.")
	pkg.Document("format", "format(t, layout?)", "The time t as UTC RFC 3339 text, or in a Go layout such as \"2006-01-02\".")
	pkg.Document("parse", "parse(text, layout?)", "Milliseconds since the Unix epoch for an RFC 3339 time, or one in a Go layout.")
	pkg.Document("second", "second", "One second, in milliseconds.")
	pkg.Document("minute", "minute", "One minute, in milliseconds.")
	pkg.Document("hour", "hour", "One hour, in milliseconds.")
	pkg.Document("day", "day", "One day, in milliseconds.")
	return pkg
}

func millis(t time.Time) float64 { return float64(t.UnixMicro()) / 1000 }

func fromMillis(ms float64) (time.Time, error) {
	if math.IsNaN(ms) || math.IsInf(ms, 0) {
		return time.Time{}, fmt.Errorf("timestamp must be a finite number, got %v", ms)
	}
	us := ms * 1000
	if us < float64(math.MinInt64) || us >= -float64(math.MinInt64) {
		return time.Time{}, fmt.Errorf("timestamp %v is out of range", ms)
	}
	return time.UnixMicro(int64(us)), nil
}
