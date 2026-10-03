package vm

import (
	"unsafe"
)

// Typed wrappers turn ordinary Go functions into natives without reflection.
// The conversion for each parameter and result type is chosen once, when the
// wrapper is built, so a call costs a direct Go call plus the boxing of its
// result. Supported parameter types are Value, float64, int, int64, string,
// bool, []byte, *Array, map[string]Value and any; results may also be an
// Evie-boxable Go value through BoxAny.

// same reinterprets u as T; callers only use it when T and U are the same type
func same[T, U any](u U) T {
	return *(*T)(unsafe.Pointer(&u))
}

type argConv[T any] func(c Call, i int) (T, error)

// argOf picks the conversion for parameters of type T
func argOf[T any]() argConv[T] {
	var zero T
	switch any(zero).(type) {
	case Value:
		return func(c Call, i int) (T, error) { return same[T](c.Arg(i)), nil }
	case float64:
		return func(c Call, i int) (T, error) { f, err := c.Number(i); return same[T](f), err }
	case int:
		return func(c Call, i int) (T, error) { n, err := c.Int(i); return same[T](n), err }
	case int64:
		return func(c Call, i int) (T, error) { n, err := c.Int(i); return same[T](int64(n)), err }
	case string:
		return func(c Call, i int) (T, error) { s, err := c.String(i); return same[T](s), err }
	case bool:
		return func(c Call, i int) (T, error) { b, err := c.Bool(i); return same[T](b), err }
	case []byte:
		return func(c Call, i int) (T, error) { b, err := c.Buffer(i); return same[T](b), err }
	case *Array:
		return func(c Call, i int) (T, error) { a, err := c.Array(i); return same[T](a), err }
	case map[string]Value:
		return func(c Call, i int) (T, error) { o, err := c.Object(i); return same[T](o), err }
	}
	if _, isAny := any(&zero).(*any); isAny {
		return func(c Call, i int) (T, error) { return same[T](c.Arg(i).ToGo()), nil }
	}
	// anything else must be a registered host type
	return func(c Call, i int) (T, error) { return HostArg[T](c, i) }
}

type retConv[T any] func(T) (Value, error)

// retOf picks the conversion for results of type T
func retOf[T any]() retConv[T] {
	var zero T
	switch any(zero).(type) {
	case Value:
		return func(r T) (Value, error) { return same[Value](r), nil }
	case float64:
		return func(r T) (Value, error) { return BoxNumber(same[float64](r)), nil }
	case int:
		return func(r T) (Value, error) { return BoxNumber(float64(same[int](r))), nil }
	case int64:
		return func(r T) (Value, error) { return BoxNumber(float64(same[int64](r))), nil }
	case string:
		return func(r T) (Value, error) { return BoxString(same[string](r)), nil }
	case bool:
		return func(r T) (Value, error) { return BoxBool(same[bool](r)), nil }
	case []byte:
		return func(r T) (Value, error) { return BoxBuffer(same[[]byte](r)), nil }
	case *Array:
		return func(r T) (Value, error) { return BoxArray(same[*Array](r)), nil }
	case map[string]Value:
		return func(r T) (Value, error) { return BoxObject(same[map[string]Value](r)), nil }
	}
	return func(r T) (Value, error) { return BoxAny(r) }
}

// Func0 wraps a function of no arguments
func Func0[R any](f func() R) *GoFunc {
	ret := retOf[R]()
	return NewFunc("native", 0, func(c Call) (Value, error) {
		return ret(f())
	})
}

// Func1 wraps a function of one argument
func Func1[A, R any](f func(A) R) *GoFunc {
	a, ret := argOf[A](), retOf[R]()
	return NewFunc("native", 1, func(c Call) (Value, error) {
		x, err := a(c, 0)
		if err != nil {
			return Value{}, err
		}
		return ret(f(x))
	})
}

// Func2 wraps a function of two arguments
func Func2[A, B, R any](f func(A, B) R) *GoFunc {
	a, b, ret := argOf[A](), argOf[B](), retOf[R]()
	return NewFunc("native", 2, func(c Call) (Value, error) {
		x, err := a(c, 0)
		if err != nil {
			return Value{}, err
		}
		y, err := b(c, 1)
		if err != nil {
			return Value{}, err
		}
		return ret(f(x, y))
	})
}

// Func3 wraps a function of three arguments
func Func3[A, B, C, R any](f func(A, B, C) R) *GoFunc {
	a, b, d, ret := argOf[A](), argOf[B](), argOf[C](), retOf[R]()
	return NewFunc("native", 3, func(c Call) (Value, error) {
		x, err := a(c, 0)
		if err != nil {
			return Value{}, err
		}
		y, err := b(c, 1)
		if err != nil {
			return Value{}, err
		}
		z, err := d(c, 2)
		if err != nil {
			return Value{}, err
		}
		return ret(f(x, y, z))
	})
}

// Func0E wraps a function of no arguments that can fail
func Func0E[R any](f func() (R, error)) *GoFunc {
	ret := retOf[R]()
	return NewFunc("native", 0, func(c Call) (Value, error) {
		r, err := f()
		if err != nil {
			return Value{}, err
		}
		return ret(r)
	})
}

// Func1E wraps a function of one argument that can fail
func Func1E[A, R any](f func(A) (R, error)) *GoFunc {
	a, ret := argOf[A](), retOf[R]()
	return NewFunc("native", 1, func(c Call) (Value, error) {
		x, err := a(c, 0)
		if err != nil {
			return Value{}, err
		}
		r, err := f(x)
		if err != nil {
			return Value{}, err
		}
		return ret(r)
	})
}

// Func2E wraps a function of two arguments that can fail
func Func2E[A, B, R any](f func(A, B) (R, error)) *GoFunc {
	a, b, ret := argOf[A](), argOf[B](), retOf[R]()
	return NewFunc("native", 2, func(c Call) (Value, error) {
		x, err := a(c, 0)
		if err != nil {
			return Value{}, err
		}
		y, err := b(c, 1)
		if err != nil {
			return Value{}, err
		}
		r, err := f(x, y)
		if err != nil {
			return Value{}, err
		}
		return ret(r)
	})
}

// Func3E wraps a function of three arguments that can fail
func Func3E[A, B, C, R any](f func(A, B, C) (R, error)) *GoFunc {
	a, b, d, ret := argOf[A](), argOf[B](), argOf[C](), retOf[R]()
	return NewFunc("native", 3, func(c Call) (Value, error) {
		x, err := a(c, 0)
		if err != nil {
			return Value{}, err
		}
		y, err := b(c, 1)
		if err != nil {
			return Value{}, err
		}
		z, err := d(c, 2)
		if err != nil {
			return Value{}, err
		}
		r, err := f(x, y, z)
		if err != nil {
			return Value{}, err
		}
		return ret(r)
	})
}

// Native binds an already built native under a name
func (pkg *Package) Native(name string, fn *GoFunc) *GoFunc {
	fn.Name = name
	pkg.Set(name, BoxGoFunc(fn))
	return fn
}
