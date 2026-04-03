package vm

type BuiltinType struct {
	Name        string
	Constructor GoFunc
}

var stringType = BoxBuiltinType("string", func(a Value) (Value, *Exception) {
	return BoxString(a.String()), nil
})

var builtins = map[string]*Value{
	"string": &stringType,
}
