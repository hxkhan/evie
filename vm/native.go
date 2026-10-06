package vm

import (
	"context"
	"fmt"
	"math"

	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/vm/fields"
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
	return *c.fbr.stack[c.base+i]
}

// This returns the receiver of a method call
func (c Call) This() Value {
	return c.Arg(0)
}

// Args copies all arguments into a new slice
func (c Call) Args() []Value {
	args := make([]Value, c.n)
	for i := range args {
		args[i] = *c.fbr.stack[c.base+i]
	}
	return args
}

// Context is done when the script has to stop: the host called Interrupt or
// the context of the run was cancelled. Blocking natives that wait should
// watch it, and returning its cause, or any error, once it is done stops the
// script right away.
func (c Call) Context() context.Context {
	return c.fbr.stop.ctx
}

// Call invokes an evie callable on the same fiber, as a callback
func (c Call) Call(fn Value, args ...Value) (Value, error) {
	if ufn, ok := fn.asUserFn(); ok {
		if exc := ufn.accepts(len(args)); exc != nil {
			return Value{}, exc
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
	return fmt.Errorf("%s: argument %d must be %s, got %s", name, i+1, want, c.Arg(i).TypeOf())
}

// Number returns argument i as a number
func (c Call) Number(i int) (float64, error) {
	if f, ok := c.Arg(i).AsFloat64(); ok {
		return f, nil
	}
	return 0, c.typeError(i, "a number")
}

// Int returns argument i as an integer; a number with a fraction is an error
func (c Call) Int(i int) (int, error) {
	f, ok := c.Arg(i).AsFloat64()
	if !ok {
		return 0, c.typeError(i, "an integer")
	}
	if f != math.Trunc(f) {
		return 0, c.typeError(i, "an integer")
	}
	return int(f), nil
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
