package vm

import (
	"hxkhan.dev/evie/ast"
	"hxkhan.dev/evie/vm/fields"
)

// evaluate can return values of type (Value, Global, local) or nil
func (vm *Instance) evaluate(node ast.Node) any {
	if !vm.cp.inline {
		return nil
	}

	switch node := node.(type) {
	case ast.Input[bool]:
		return BoxBool(node.Value)

	case ast.Input[float64]:
		return BoxNumber(node.Value)

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
		if lhs, ok := vm.evaluate(node.Lhs).(Value); ok {
			if field, exists := lhs.getField(fields.Get(node.Rhs)); exists {
				//fmt.Println(node, "->", field)
				return field
			}
		}

	case ast.BinOp:
		if lhs, ok := vm.evaluate(node.Lhs).(Value); ok {
			if rhs, ok := vm.evaluate(node.Rhs).(Value); ok {
				switch node.Operator {
				case ast.AddOp:
					if res, ok := lhs.Add(rhs); ok {
						return res
					}
				case ast.SubOp:
					if res, ok := lhs.Sub(rhs); ok {
						return res
					}
				case ast.MulOp:
					if res, ok := lhs.Mul(rhs); ok {
						return res
					}
				case ast.DivOp:
					if res, ok := lhs.Div(rhs); ok {
						return res
					}
				case ast.ModOp:
					if res, ok := lhs.Mod(rhs); ok {
						return res
					}
				case ast.EqOp:
					return BoxBool(lhs.Equals(rhs))
				case ast.LtOp:
					if res, ok := lhs.LessThan(rhs); ok {
						return res
					}
				case ast.GtOp:
					if res, ok := lhs.GreaterThan(rhs); ok {
						return res
					}
				}
			}
		}
	}

	return nil
}
