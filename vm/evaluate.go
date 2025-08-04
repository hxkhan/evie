package vm

import (
	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/vm/fields"
)

func (vm *Instance) evaluate(node ast.Node) (Value, bool) {
	switch node := node.(type) {
	case ast.Input[bool]:
		return BoxBool(node.Value), true

	case ast.Input[float64]:
		return BoxFloat64(node.Value), true

	case ast.Input[string]:
		return BoxString(node.Value), true

	case ast.Input[struct{}]:
		return Value{}, true

	case ast.Ident:
		variable, err := vm.cp.reach(node.Name)
		if err != nil {
			panic(err)
		}

		if global, isGlobal := variable.(Global); isGlobal && global.IsStatic {
			return *(global.Value), true
		}

		return Value{}, false

	case ast.FieldAccess:
		if lhs, ok := vm.evaluate(node.Lhs); ok {
			if field, exists := lhs.getField(fields.Get(node.Rhs)); exists {
				return field, true
			}
		}
	}

	return Value{}, false
}
