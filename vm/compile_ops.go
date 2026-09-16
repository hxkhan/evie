package vm

import (
	"fmt"

	"github.com/hxkhan/evie/ast"
)

func (vm *Instance) emitNeg(node ast.Neg) instruction {
	if val, ok := vm.evaluate(node).(Value); ok {
		return func(fbr *Fiber) (Value, Exception) {
			return val, nil
		}
	}

	value := vm.compile(node.Value)

	return func(fbr *Fiber) (Value, Exception) {
		value, exc := value(fbr)
		if exc != nil {
			return value, exc
		}

		if float, ok := value.AsFloat64(); ok {
			return BoxNumber(-float), nil
		}
		return Value{}, RuntimeExceptionF("Cannot negate '%v'.", value)
	}
}

func (vm *Instance) emitNot(node ast.Not) instruction {
	if val, ok := vm.evaluate(node).(Value); ok {
		return func(fbr *Fiber) (Value, Exception) {
			return val, nil
		}
	}

	value := vm.compile(node.Value)

	return func(fbr *Fiber) (Value, Exception) {
		value, exc := value(fbr)
		if exc != nil {
			return value, exc
		}

		return BoxBool(!value.IsTruthy()), nil
	}
}

func (vm *Instance) emitCatch(node ast.Catch) instruction {
	action := vm.compile(node.Action)

	return func(fbr *Fiber) (Value, Exception) {
		v, err := action(fbr)
		if err != nil {
			if err != signalReturn && err != signalContinue && err != signalBreak {
				// convert exception to error-as-value and return
				return BoxError(err), nil
			}

			// propagate as normal
			return v, err
		}

		return v, nil
	}
}

func (vm *Instance) emitBinOp(node ast.BinOp) instruction {
	if val, ok := vm.evaluate(node).(Value); ok {
		return func(fbr *Fiber) (Value, Exception) {
			return val, nil
		}
	}

	/*
		1. Local x Local
		2. Local x Value
		3. Value x Local

		With these three, we cover all possible combinations of binary operations
	*/

	if lhs := vm.evaluate(node.Lhs); lhs != nil {
		if rhs := vm.evaluate(node.Rhs); rhs != nil {
			// optimise: lhs being a local
			if lhs, isLocal := lhs.(local); isLocal {
				// optimise: rhs being a local
				if rhs, isLocal := rhs.(local); isLocal {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.Add(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("+", lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.Sub(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("-", lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.Mul(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("*", lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.Div(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("/", lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.Mod(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("%", lhs, rhs)
						}

					case ast.EqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							return BoxBool(lhs.Equals(rhs)), nil
						}

					case ast.NotEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							return BoxBool(!lhs.Equals(rhs)), nil
						}

					case ast.LtOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.LessThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.GtOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.GreaterThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError(">", lhs, rhs)
						}

					case ast.LtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.LessThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<=", lhs, rhs)
						}

					case ast.GtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs, rhs := *fbr.get(lhs), *fbr.get(rhs)
							if result, ok := lhs.GreaterThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError(">=", lhs, rhs)
						}

					case ast.OrOp:
						return func(fbr *Fiber) (Value, Exception) {
							if fbr.get(lhs).IsTruthy() {
								return BoxBool(true), nil
							} else if fbr.get(rhs).IsTruthy() {
								return BoxBool(true), nil
							}
							return BoxBool(false), nil
						}

					case ast.AndOp:
						return func(fbr *Fiber) (Value, Exception) {
							return BoxBool(fbr.get(lhs).IsTruthy() && fbr.get(rhs).IsTruthy()), nil
						}
					}
				}

				// optimise: rhs being a constant
				if rhs, isValue := rhs.(Value); isValue {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.Add(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("+", lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.Sub(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("-", lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.Mul(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("*", lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.Div(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("/", lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.Mod(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("%", lhs, rhs)
						}

					case ast.EqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							return BoxBool(lhs.Equals(rhs)), nil
						}

					case ast.NotEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							return BoxBool(!lhs.Equals(rhs)), nil
						}

					case ast.LtOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.LessThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.GtOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.GreaterThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.LtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.LessThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<=", lhs, rhs)
						}

					case ast.GtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := *fbr.get(lhs)
							if result, ok := lhs.GreaterThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError(">=", lhs, rhs)
						}
					}
				}
			}

			// optimise: lhs being a constant
			if lhs, isLocal := lhs.(Value); isLocal {
				if rhs, isValue := rhs.(local); isValue {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Add(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("+", lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Sub(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("-", lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Mul(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("*", lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Div(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("/", lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Mod(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("%", lhs, rhs)
						}

					case ast.EqOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							return BoxBool(lhs.Equals(rhs)), nil
						}

					case ast.NotEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							return BoxBool(!lhs.Equals(rhs)), nil
						}

					case ast.LtOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.LessThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.GtOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.GreaterThan(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<", lhs, rhs)
						}

					case ast.LtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.LessThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError("<=", lhs, rhs)
						}

					case ast.GtEqOp:
						return func(fbr *Fiber) (Value, Exception) {
							rhs := *fbr.get(rhs)
							if result, ok := lhs.GreaterThanOrEqualTo(rhs); ok {
								return result, nil
							}
							return Value{}, operatorError(">=", lhs, rhs)
						}
					}
				}
			}
		}
	}

	lhs := vm.compile(node.Lhs)
	rhs := vm.compile(node.Rhs)

	// generic compilation
	switch node.Operator {
	case ast.AddOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Add(b); ok {
				return result, nil
			}
			return Value{}, operatorError("+", a, b)
		}

	case ast.SubOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Sub(b); ok {
				return result, nil
			}
			return Value{}, operatorError("-", a, b)
		}

	case ast.MulOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Mul(b); ok {
				return result, nil
			}
			return Value{}, operatorError("*", a, b)
		}

	case ast.DivOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Div(b); ok {
				return result, nil
			}
			return Value{}, operatorError("/", a, b)
		}

	case ast.ModOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.Mod(b); ok {
				return result, nil
			}
			return Value{}, operatorError("%", a, b)
		}

	case ast.EqOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			return BoxBool(a.Equals(b)), nil
		}

	case ast.NotEqOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			return BoxBool(!a.Equals(b)), nil
		}

	case ast.LtOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.LessThan(b); ok {
				return result, nil
			}
			return Value{}, operatorError("<", a, b)
		}

	case ast.GtOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.GreaterThan(b); ok {
				return result, nil
			}
			return Value{}, operatorError(">", a, b)
		}

	case ast.LtEqOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.LessThanOrEqualTo(b); ok {
				return result, nil
			}
			return Value{}, operatorError("<=", a, b)
		}

	case ast.GtEqOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			if result, ok := a.GreaterThanOrEqualTo(b); ok {
				return result, nil
			}
			return Value{}, operatorError(">=", a, b)
		}

	case ast.OrOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			if a.IsTruthy() {
				return BoxBool(true), nil
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			return BoxBool(b.IsTruthy()), nil
		}

	case ast.AndOp:
		return func(fbr *Fiber) (Value, Exception) {
			a, err := lhs(fbr)
			if err != nil {
				return a, err
			}
			if !a.IsTruthy() {
				return BoxBool(false), nil
			}
			b, err := rhs(fbr)
			if err != nil {
				return a, err
			}
			return BoxBool(b.IsTruthy()), nil
		}
	}

	panic(fmt.Errorf("implement Operator(%v)", node.Operator))
}

func (vm *Instance) emitMutableBinOp(node ast.MutableBinOp) instruction {
	if lhs := vm.evaluate(node.Lhs); lhs != nil {
		// optimise: lhs being a local
		if lhs, ok := lhs.(local); ok {
			if lhs.isStatic {
				panic(CompileErrorf("Assignment to constant binding on line '%v'.", node.Line()))
			}

			if rhs := vm.evaluate(node.Rhs); rhs != nil {
				// optimise: rhs being a local
				if rhs, ok := rhs.(local); ok {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Add(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("+", *lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Sub(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("-", *lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Mul(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("*", *lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Div(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("/", *lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							rhs := *fbr.get(rhs)
							if result, ok := lhs.Mod(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("%", *lhs, rhs)
						}
					}
				}

				// optimise: rhs being a constant
				if rhs, ok := rhs.(Value); ok {
					switch node.Operator {
					case ast.AddOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Add(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("+", *lhs, rhs)
						}

					case ast.SubOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Sub(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("-", *lhs, rhs)
						}

					case ast.MulOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Mul(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("*", *lhs, rhs)
						}

					case ast.DivOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Div(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("/", *lhs, rhs)
						}

					case ast.ModOp:
						return func(fbr *Fiber) (Value, Exception) {
							lhs := fbr.get(lhs)
							if result, ok := lhs.Mod(rhs); ok {
								*lhs = result
								return Value{}, nil
							}
							return Value{}, operatorError("%", *lhs, rhs)
						}
					}
				}
			}

			// generic rhs compilation for local lhs
			rhs := vm.compile(node.Rhs)
			switch node.Operator {
			case ast.AddOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					lhs := fbr.get(lhs)
					if result, ok := lhs.Add(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("+", *lhs, rhs)
				}

			case ast.SubOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					lhs := fbr.get(lhs)
					if result, ok := lhs.Sub(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("-", *lhs, rhs)
				}

			case ast.MulOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					lhs := fbr.get(lhs)
					if result, ok := lhs.Mul(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("*", *lhs, rhs)
				}

			case ast.DivOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					lhs := fbr.get(lhs)
					if result, ok := lhs.Div(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("/", *lhs, rhs)
				}

			case ast.ModOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					lhs := fbr.get(lhs)
					if result, ok := lhs.Mod(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("%", *lhs, rhs)
				}
			}
		}

		// optimise: lhs being a global
		if lhs, ok := lhs.(Global); ok {
			if lhs.IsStatic {
				panic(CompileErrorf("Assignment to constant binding on line '%v'.", node.Line()))
			}

			lhs := lhs.Value
			rhs := vm.compile(node.Rhs)
			switch node.Operator {
			case ast.AddOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					if result, ok := lhs.Add(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("+", *lhs, rhs)
				}

			case ast.SubOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					if result, ok := lhs.Sub(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("-", *lhs, rhs)
				}

			case ast.MulOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					if result, ok := lhs.Mul(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("*", *lhs, rhs)
				}

			case ast.DivOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					if result, ok := lhs.Div(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("/", *lhs, rhs)
				}

			case ast.ModOp:
				return func(fbr *Fiber) (Value, Exception) {
					rhs, err := rhs(fbr)
					if err != nil {
						return rhs, err
					}
					if result, ok := lhs.Mod(rhs); ok {
						*lhs = result
						return Value{}, nil
					}
					return Value{}, operatorError("%", *lhs, rhs)
				}
			}
		}
	}

	panic(fmt.Errorf("implement Operator(%v) in %v", node.Operator, node))
}
