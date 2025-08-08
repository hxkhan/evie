package vm

import (
	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/vm/fields"
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
					if a, ok := lhs.AsFloat64(); ok {
						if b, ok := rhs.AsFloat64(); ok {
							return BoxFloat64(a + b)
						}
					}

					if a, ok := lhs.AsString(); ok {
						if b, ok := rhs.AsString(); ok {
							return BoxString(a + b)
						}
					}

				case ast.SubOp:
					if a, ok := lhs.AsFloat64(); ok {
						if b, ok := rhs.AsFloat64(); ok {
							return BoxFloat64(a - b)
						}
					}

				case ast.MulOp:
					if a, ok := lhs.AsFloat64(); ok {
						if b, ok := rhs.AsFloat64(); ok {
							return BoxFloat64(a * b)
						}
					}

				case ast.DivOp:
					if a, ok := lhs.AsFloat64(); ok {
						if b, ok := rhs.AsFloat64(); ok {
							return BoxFloat64(a / b)
						}
					}
				case ast.ModOp:
					if a, ok := lhs.AsFloat64(); ok {
						if b, ok := rhs.AsFloat64(); ok {
							return BoxFloat64(float64(int64(a) % int64(b)))
						}
					}

				case ast.EqOp:
					return BoxBool(lhs.Equals(rhs))

				case ast.LtOp:
					if a, ok := lhs.AsFloat64(); ok {
						if b, ok := rhs.AsFloat64(); ok {
							return BoxBool(a < b)
						}
					}

				case ast.GtOp:
					if a, ok := lhs.AsFloat64(); ok {
						if b, ok := rhs.AsFloat64(); ok {
							return BoxBool(a > b)
						}
					}
				}
			}
		}
	}

	return nil
}
