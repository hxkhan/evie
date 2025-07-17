package core

// compile time safety so uncallable functions don't get into the system
type GoFunc interface {
	func() (Value, error) |
		func(Value) (Value, error) |
		func(Value, Value) (Value, error) |
		func(Value, Value, Value) (Value, error) |
		func(Value, Value, Value, Value) (Value, error) |
		func(Value, Value, Value, Value, Value) (Value, error) |
		func(Value, Value, Value, Value, Value, Value) (Value, error)
}

/* type NativeFn struct {
	Callable any
}

func (fn NativeFn) String() string {
	return "<fn>"
}

func (fn NativeFn) TypeOf() string {
	return "function"
}

func (fn NativeFn) Name() string {
	path := runtime.FuncForPC(reflect.ValueOf(fn.Callable).Pointer()).Name()
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

func (fn NativeFn) Nargs() int {
	return reflect.TypeOf(fn.Callable).NumIn()
} */
