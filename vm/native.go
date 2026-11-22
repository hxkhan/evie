package vm

import (
	"context"
	"fmt"
	"math"

	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/internal/fields"
)

// NativeFn is a Go function exposed to evie code. It receives its arguments
// through the Call and returns a value or an error; an error becomes an evie
// exception the script can catch.
type NativeFn = func(c Call) (Value, error)

// Call is what a native receives: the arguments of one invocation, living on
// the fiber that made it, and a way to call back into evie on that fiber. It
// is passed by value and costs nothing to create.
type Call struct {
	fbr  *fiber
	fn   *GoFunc
	base int
	n    int
}

// Len returns the number of arguments, the receiver included for a method
func (c Call) Len() int {
	return c.n
}

// Arg returns argument i, or nil when there is no such argument
func (c Call) Arg(i int) Value {
	if i < 0 || i >= c.n {
		return Value{}
	}
	return c.fbr.stack[c.base+i]
}

// This returns the receiver of a method call
func (c Call) This() Value {
	return c.Arg(0)
}

// Args copies all arguments into a new slice
func (c Call) Args() []Value {
	args := make([]Value, c.n)
	copy(args, c.fbr.stack[c.base:c.base+c.n])
	return args
}

// Context is done when the script has to stop: the host called Interrupt or
// the context of the run was cancelled. Blocking natives that wait should
// watch it, and returning its cause, or any error, once it is done stops the
// script right away.
func (c Call) Context() context.Context {
	return c.fbr.stop.ctx
}

// Async runs fn on its own goroutine tied to the calling instance's lifecycle
// and cancellation context, returning a Task scripts can await. Engine.Close
// waits for the task, task.cancel() or Interrupt() cancels ctx, and panics
// become task errors rather than crashing the host.
func (c Call) Async(fn func(ctx context.Context) (Value, error)) Value {
	task := newTaskInstance()
	if c.fbr.vm == nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					task.reject(recovered(r))
				}
			}()
			res, err := fn(c.Context())
			task.finish(res, toException(err))
		}()
		return boxTask(task)
	}
	stop := c.fbr.vm.rt.newTaskStopper(c.fbr.stop)
	task.stop = stop
	c.fbr.vm.rt.wg.Go(func() {
		defer stop.drop()
		defer func() {
			if r := recover(); r != nil {
				task.reject(recovered(r))
			}
		}()
		res, err := fn(stop.ctx)
		if stop.done() && err == nil {
			task.reject(stop.failure())
			return
		}
		task.finish(res, toException(err))
	})
	return boxTask(task)
}

// Call invokes an evie callable on the same fiber, as a callback
func (c Call) Call(fn Value, args ...Value) (Value, error) {
	if c.fbr.vm == nil {
		return plain(fn.callGuarded(nil, args...))
	}
	if ufn, ok := fn.asUserFn(); ok {
		if exc := ufn.accepts(len(args)); exc != nil {
			return Value{}, exc
		}
		if c.fbr.lock() {
			defer c.fbr.unlock()
		}
		return plain(ufn.call(c.fbr, args))
	}
	return plain(fn.callGuarded(c.fbr, args...))
}

func (c Call) typeError(i int, want string) error {
	name := c.fn.Name
	if name == "" {
		name = "native"
	}
	pos := i + 1
	if c.fn.isMethod {
		pos = i
	}
	return fmt.Errorf("%s: argument %d must be %s, got %s", name, pos, want, c.Arg(i).TypeOf())
}

// Number returns argument i as a number
func (c Call) Number(i int) (float64, error) {
	if f, ok := c.Arg(i).AsFloat64(); ok {
		return f, nil
	}
	return 0, c.typeError(i, "a number")
}

// Int returns argument i as an integer; a number with a fraction, NaN, infinity
// or a value outside int's range is an error
func (c Call) Int(i int) (int, error) {
	f, ok := c.Arg(i).AsFloat64()
	if !ok {
		return 0, c.typeError(i, "an integer")
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) || f < math.MinInt || f >= -float64(math.MinInt) {
		return 0, c.typeError(i, "an integer")
	}
	return int(f), nil
}

// Int64 returns argument i as a 64-bit integer; a number with a fraction, NaN,
// infinity or a value outside int64's range is an error
func (c Call) Int64(i int) (int64, error) {
	f, ok := c.Arg(i).AsFloat64()
	if !ok {
		return 0, c.typeError(i, "an integer")
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) || f < math.MinInt64 || f >= -float64(math.MinInt64) {
		return 0, c.typeError(i, "an integer")
	}
	return int64(f), nil
}

// String returns argument i as a string
func (c Call) String(i int) (string, error) {
	if s, ok := c.Arg(i).AsString(); ok {
		return s, nil
	}
	return "", c.typeError(i, "a string")
}

// Bool returns argument i as a boolean
func (c Call) Bool(i int) (bool, error) {
	if b, ok := c.Arg(i).AsBool(); ok {
		return b, nil
	}
	return false, c.typeError(i, "a bool")
}

// Array returns argument i as an array
func (c Call) Array(i int) (*Array, error) {
	if arr, ok := c.Arg(i).AsArray(); ok {
		return arr, nil
	}
	return nil, c.typeError(i, "an array")
}

// Object returns argument i as an object
func (c Call) Object(i int) (map[string]Value, error) {
	if obj, ok := c.Arg(i).AsObject(); ok {
		return obj, nil
	}
	return nil, c.typeError(i, "an object")
}

// Buffer returns argument i as a buffer
func (c Call) Buffer(i int) ([]byte, error) {
	if buf, ok := c.Arg(i).AsBuffer(); ok {
		return buf, nil
	}
	return nil, c.typeError(i, "a buffer")
}

// Callable returns argument i if it can be called
func (c Call) Callable(i int) (Value, error) {
	if v := c.Arg(i); v.IsCallable() {
		return v, nil
	}
	return Value{}, c.typeError(i, "callable")
}

// plain turns an exception into a plain error without the typed-nil trap
func plain(v Value, exc exception) (Value, error) {
	if exc == nil {
		return v, nil
	}
	return v, exc
}

// toException converts an error returned by a native into an exception
func toException(err error) exception {
	if err == nil {
		return nil
	}
	if exc, ok := err.(*Error); ok {
		return exc
	}
	return &Error{Report: diag.Report{Message: err.Error()}, Cause: err}
}

// NewFunc describes a native taking exactly arity arguments
func NewFunc(name string, arity int, fn NativeFn) *GoFunc {
	return &GoFunc{Name: name, arguments: arity, fn: fn}
}

// NewVariadic describes a native taking at least min arguments
func NewVariadic(name string, min int, fn NativeFn) *GoFunc {
	return &GoFunc{Name: name, arguments: min, isVariadic: true, fn: fn}
}

// newMethod describes a native method; arity counts the arguments after the receiver
func newMethod(name string, arity int, fn NativeFn) *GoFunc {
	return &GoFunc{Name: name, arguments: 1 + arity, isMethod: true, fn: fn}
}

// newVariadicMethod describes a native method taking at least min arguments after the receiver
func newVariadicMethod(name string, min int, fn NativeFn) *GoFunc {
	return &GoFunc{Name: name, arguments: 1 + min, isVariadic: true, isMethod: true, fn: fn}
}

// methodTable builds the member table of a type from its methods
func methodTable(fns ...*GoFunc) map[fields.ID]Value {
	table := make(map[fields.ID]Value, len(fns))
	for _, fn := range fns {
		table[fields.Get(fn.Name)] = BoxGoFunc(fn)
	}
	return table
}
