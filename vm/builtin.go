package vm

import "github.com/hxkhan/evie/ast"

var builtins = map[string]*Value{
	"string": BoxGoFunc(func(a Value) (Value, *Exception) {
		return BoxString(a.String()), nil
	}, ast.UndefinedMode).Allocate(),
}
