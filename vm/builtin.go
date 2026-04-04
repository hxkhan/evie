package vm

type BuiltinType struct {
	Name        string
	Constructor GoFunc
}

var stringType = BoxBuiltinType("string", func(x Value) (Value, Exception) {
	return BoxString(x.String()), nil
})

var builtins = map[string]*Value{
	"string": stringType.Allocate(),
	"error":  BoxUserStruct(exception).Allocate(),
}
