package core

import (
	"reflect"
	"runtime"
	"strings"

	"github.com/hk-32/evie/box"
)

// compile time safety so uncallable functions don't get into the system
type ValidFnTypes interface {
	func() (box.Value, error) |
		func(box.Value) (box.Value, error) |
		func(box.Value, box.Value) (box.Value, error) |
		func(box.Value, box.Value, box.Value) (box.Value, error) |
		func(box.Value, box.Value, box.Value, box.Value) (box.Value, error) |
		func(box.Value, box.Value, box.Value, box.Value, box.Value) (box.Value, error) |
		func(box.Value, box.Value, box.Value, box.Value, box.Value, box.Value) (box.Value, error)
}

type NativeFn[T ValidFnTypes] struct {
	Callable T
}

func (fn NativeFn[T]) String() string {
	return "<fn>"
}

func (fn NativeFn[T]) TypeOf() string {
	return "function"
}

func (fn NativeFn[T]) Name() string {
	path := runtime.FuncForPC(reflect.ValueOf(fn.Callable).Pointer()).Name()
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func (fn NativeFn[T]) Nargs() int {
	return reflect.TypeOf(fn.Callable).NumIn()
}
