package vm

import (
	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/vm/fields"
)

// evaluate can return values of type (Value, Global, local) or nil
func (vm *Instance) evaluate(node ast.Node) any {
	switch node := node.(type) {
	case ast.Input[bool]:
		return BoxBool(node.Value)

	case ast.Input[float64]:
		return BoxFloat64(node.Value)

	case ast.Input[string]:
		return BoxString(node.Value)

	case ast.Input[struct{}]:
		return Value{}

	case ast.Ident:
		variable, err := vm.cp.reach(node.Name)
		if err != nil {
			panic(err)
		}

		if global, isGlobal := variable.(Global); isGlobal {
			// global statics evaluate to Value instead of Global
			if global.IsStatic {
				return *(global.Value)
			}
			return global
		} else if local, isLocal := variable.(local); isLocal {
			return local
		}

		return nil

	case ast.FieldAccess:
		if lhs := vm.evaluate(node.Lhs); lhs != nil {
			if lhs, isValue := lhs.(Value); isValue {
				if field, exists := lhs.getField(fields.Get(node.Rhs)); exists {
					return field
				}
			}
		}
	}

	return nil
}
