package vm

var builtins = map[string]*Value{
	"string": BoxGoFunc(func(a Value) (Value, *Exception) {
		return BoxString(a.String()), nil
	}).Allocate(),
}
