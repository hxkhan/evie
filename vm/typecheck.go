package vm

import (
	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/types"
)

func (vm *Instance) typecheck(node ast.Node) types.Type {
	switch node := node.(type) {
	case ast.Input[bool]:
		return types.Bool
	case ast.Input[float64]:
		return types.Float
	case ast.Input[string]:
		return types.String
	case ast.Input[struct{}]:
		return types.Nil

	case ast.Ident:
		v, err := vm.cp.reach(node.Name)
		if err != nil {
			panic(err)
		}

		if global, isGlobal := v.(binding[Global]); isGlobal {
			return global.Type
		} else if local, isLocal := v.(binding[local]); isLocal {
			return local.Type
		}

		panic("unknown type")

	/* case ast.FieldAccess:
	if lhs, ok := vm.evaluate(node.Lhs).(Value); ok {
		if v, exists := lhs.getField(fields.Get(node.Rhs)); exists {
			return v
		}
	} */

	case ast.BinOp:
		if lhs := vm.typecheck(node.Lhs); lhs != nil {
			if rhs := vm.typecheck(node.Rhs); rhs != nil {
				switch node.Operator {
				case ast.AddOp, ast.SubOp, ast.MulOp, ast.DivOp:
					if lhs.Equals(rhs) {
						return lhs
					}
					panic("math between incompatible types")

				case ast.ModOp:
					if lhs.Equals(rhs) {
						return lhs
					}
					panic("modulo between incompatible types")

				case ast.EqOp:
					if lhs.Equals(rhs) {
						return types.Bool
					}
					panic("equality between incompatible types")

				case ast.LtOp, ast.GtOp:
					if lhs.Equals(rhs) {
						return types.Bool
					}
					panic("comparison between incompatible types")
				}
			}
		}

	case ast.Call:
		fn, ok := vm.typecheck(node.Fn).(*types.FnType)
		if !ok {
			panic("function type not found")
		}

		if len(node.Args) != len(fn.Params) {
			panic("wrong number of arguments")
		}

		for i, arg := range node.Args {
			argType := vm.typecheck(arg)
			if argType == nil {
				panic("argument type not found")
			}

			if !argType.Equals(fn.Params[i]) {
				panic("argument type mismatch")
			}
		}

		return fn.Return
	}

	return nil
}
