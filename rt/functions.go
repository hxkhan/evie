package rt

import "fmt"

// Assert fn to a specific type
func AsFn[T FnTypes](id string, fn any) T {
	if fun, isFn := fn.(Fn[T]); isFn {
		return fun.Obj
	}
	panic(TypeError("Cannot call '%v' of type '%T'", id, fn))
}

func Is[T any](fn any) (T, bool) {
	fun, isFn := fn.(T)
	return fun, isFn
}

func Go(fn func()) {
	go func() {
		GIL.Lock()
		fn()
		GIL.Unlock()
	}()
}

// compile time safety so uncallable functions don't get into the system
type FnTypes interface {
	func() any |
		func(any) any |
		func(any, any) any |
		func(any, any, any) any |
		func(any, any, any, any) any |
		func(any, any, any, any, any) any |
		func(any, any, any, any, any, any) any
}

type Fn[T FnTypes] struct {
	Name string
	Obj  T
}

func (fn Fn[T]) String() string {
	return fmt.Sprintf("<fn %s>", fn.Name)
}

func (fn Fn[T]) TypeOf() string {
	return "function"
}

func (fn Fn[T]) NameOf() string {
	return fn.Name
}

func (fn Fn[T]) Procedure() any {
	return fn.Obj
}

// Helper function to create a function
func NewFn[T FnTypes](name string, fn T) Fn[T] {
	return Fn[T]{name, fn}
}
