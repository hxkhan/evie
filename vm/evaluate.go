package vm

import (
	"strings"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/internal/fields"
)

// evaluate folds constants and resolves names while compiling. It returns a
// Value for an expression that folds to a constant, a local or a Global for a
// name, and nil for anything that depends on runtime state.
func (vm *Instance) evaluate(node ast.Node) operand {
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
		if node.Name == "super" {
			return nil
		}
		variable, err := vm.cp.reach(node.Name)
		if err != nil {
			panic(vm.locate(err, node))
		}

		// a constant global evaluates to its value, anything else to the binding
		if global, isGlobal := variable.(pkgGlobal); isGlobal && global.IsConstant {
			return *global.Value
		}
		return variable

	case ast.FieldAccess:
		if lhs, ok := vm.evaluate(node.Lhs).(Value); ok {
			// only fold member reads on packages (public constants) and builtin types;
			// never freeze fields of mutable containers like objects or struct instances
			if pkg, ok := lhs.AsPackage(); ok {
				if global, exists := pkg.globals[fields.Get(node.Rhs)]; !exists || !global.IsPublic || !global.IsConstant {
					return nil
				}
				return *pkg.globals[fields.Get(node.Rhs)].Value
			}
			if _, ok := lhs.asBuiltinType(); ok {
				if v, exists := lhs.getField(fieldKeyOf(node.Rhs)); exists {
					return v
				}
			}
		}

	case ast.StringTemplate:
		var b strings.Builder
		b.WriteString(node.Parts[0])
		for i, arg := range node.Args {
			val, ok := vm.evaluate(arg).(Value)
			if !ok {
				return nil
			}
			writeInterpolated(&b, val)
			b.WriteString(node.Parts[i+1])
		}
		return BoxString(b.String())

	case ast.BinOp:
		if lhs, ok := vm.evaluate(node.Lhs).(Value); ok {
			if rhs, ok := vm.evaluate(node.Rhs).(Value); ok {
				switch node.Operator {
				case ast.AddOp:
					if res, ok := lhs.add(rhs); ok {
						return res
					}
				case ast.SubOp:
					if res, ok := lhs.sub(rhs); ok {
						return res
					}
				case ast.MulOp:
					if res, ok := lhs.mul(rhs); ok {
						return res
					}
				case ast.DivOp:
					if res, ok := lhs.div(rhs); ok {
						return res
					}
				case ast.ModOp:
					if res, ok := lhs.mod(rhs); ok {
						return res
					}
				case ast.EqOp:
					return BoxBool(lhs.Equals(rhs))
				case ast.NotEqOp:
					return BoxBool(!lhs.Equals(rhs))
				case ast.LtOp:
					if res, ok := lhs.lessThan(rhs); ok {
						return res
					}
				case ast.GtOp:
					if res, ok := lhs.greaterThan(rhs); ok {
						return res
					}
				case ast.LtEqOp:
					if res, ok := lhs.lessThanOrEqualTo(rhs); ok {
						return res
					}
				case ast.GtEqOp:
					if res, ok := lhs.greaterThanOrEqualTo(rhs); ok {
						return res
					}
				case ast.AndOp:
					return BoxBool(lhs.IsTruthy() && rhs.IsTruthy())
				case ast.OrOp:
					return BoxBool(lhs.IsTruthy() || rhs.IsTruthy())
				}
			}
		}

	case ast.Not:
		if val, ok := vm.evaluate(node.Value).(Value); ok {
			return BoxBool(!val.IsTruthy())
		}

	case ast.Plus:
		if val, ok := vm.evaluate(node.Value).(Value); ok {
			if _, ok := val.AsFloat64(); ok {
				return val
			}
		}

	case ast.Neg:
		if val, ok := vm.evaluate(node.Value).(Value); ok {
			if f, ok := val.AsFloat64(); ok {
				return BoxNumber(-f)
			}
		}
	}

	return nil
}
