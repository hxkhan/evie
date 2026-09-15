package vm

import (
	"math"
	"sort"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/ds"
	"github.com/hxkhan/evie/vm/fields"
)

func (vm *Instance) emitReturn(node ast.Return) instruction {
	if vm.cp.inline {
		// optimise: returning constants
		if in, isInput := node.Value.(ast.Input[float64]); isInput {
			value := BoxNumber(in.Value)
			return func(fbr *Fiber) (Value, Exception) {
				return value, signalReturn
			}
		}

		// optimise: returning locals variable
		if iGet, isIdentGet := node.Value.(ast.Ident); isIdentGet {
			variable, err := vm.cp.reach(iGet.Name)
			if err != nil {
				panic(err)
			}

			if v, isLocal := variable.(local); isLocal {
				if v.isCaptured {
					return func(fbr *Fiber) (Value, Exception) {
						return fbr.getCaptured(v.index), signalReturn
					}
				}
				return func(fbr *Fiber) (Value, Exception) {
					return fbr.GetLocal(v.index), signalReturn
				}
			}
		}
	}

	what := vm.compile(node.Value)
	return func(fbr *Fiber) (Value, Exception) {
		v, err := what(fbr)
		if err != nil {
			return v, err
		}

		return v, signalReturn
	}
}

func (vm *Instance) emitThrow(node ast.Throw) instruction {
	what := vm.compile(node.Value)
	return func(fbr *Fiber) (Value, Exception) {
		v, err := what(fbr)
		if err != nil {
			return v, err
		}

		if e, ok := v.AsError(); ok {
			return Value{}, e
		}

		return Value{}, CustomError("%s", v.String())
	}
}

func (vm *Instance) compileCondition(node ast.Node) func(fbr *Fiber) (bool, Exception) {
	if bin, ok := node.(ast.BinOp); ok {
		switch bin.Operator {
		case ast.LtOp, ast.LtEqOp, ast.GtOp, ast.GtEqOp, ast.EqOp, ast.NotEqOp:
			// optimise: lhs local and rhs constant
			if lhsLocal, isLocal := vm.evaluate(bin.Lhs).(local); isLocal {
				if rhsVal, isVal := vm.evaluate(bin.Rhs).(Value); isVal {
					if rhsNum, isNum := rhsVal.AsFloat64(); isNum {
						switch bin.Operator {
						case ast.LtOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								if lhs.pointer == f64Type {
									return math.Float64frombits(lhs.scalar) < rhsNum, nil
								}
								res, ok := lhs.LessThan(rhsVal)
								if !ok {
									return false, operatorError("<", lhs, rhsVal)
								}
								return res.IsTruthy(), nil
							}
						case ast.LtEqOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								if lhs.pointer == f64Type {
									return math.Float64frombits(lhs.scalar) <= rhsNum, nil
								}
								res, ok := lhs.LessThanOrEqualTo(rhsVal)
								if !ok {
									return false, operatorError("<=", lhs, rhsVal)
								}
								return res.IsTruthy(), nil
							}
						case ast.GtOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								if lhs.pointer == f64Type {
									return math.Float64frombits(lhs.scalar) > rhsNum, nil
								}
								res, ok := lhs.GreaterThan(rhsVal)
								if !ok {
									return false, operatorError(">", lhs, rhsVal)
								}
								return res.IsTruthy(), nil
							}
						case ast.GtEqOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								if lhs.pointer == f64Type {
									return math.Float64frombits(lhs.scalar) >= rhsNum, nil
								}
								res, ok := lhs.GreaterThanOrEqualTo(rhsVal)
								if !ok {
									return false, operatorError(">=", lhs, rhsVal)
								}
								return res.IsTruthy(), nil
							}
						case ast.EqOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								return lhs.Equals(rhsVal), nil
							}
						case ast.NotEqOp:
							return func(fbr *Fiber) (bool, Exception) {
								lhs := *fbr.get(lhsLocal)
								return !lhs.Equals(rhsVal), nil
							}
						}
					}

					switch bin.Operator {
					case ast.EqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs := *fbr.get(lhsLocal)
							return lhs.Equals(rhsVal), nil
						}
					case ast.NotEqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs := *fbr.get(lhsLocal)
							return !lhs.Equals(rhsVal), nil
						}
					}
				}
			}

			// optimise: lhs local and rhs local
			if lhsLocal, isLocalL := vm.evaluate(bin.Lhs).(local); isLocalL {
				if rhsLocal, isLocalR := vm.evaluate(bin.Rhs).(local); isLocalR {
					switch bin.Operator {
					case ast.LtOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							if lhs.pointer == f64Type && rhs.pointer == f64Type {
								return math.Float64frombits(lhs.scalar) < math.Float64frombits(rhs.scalar), nil
							}
							res, ok := lhs.LessThan(rhs)
							if !ok {
								return false, operatorError("<", lhs, rhs)
							}
							return res.IsTruthy(), nil
						}
					case ast.LtEqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							if lhs.pointer == f64Type && rhs.pointer == f64Type {
								return math.Float64frombits(lhs.scalar) <= math.Float64frombits(rhs.scalar), nil
							}
							res, ok := lhs.LessThanOrEqualTo(rhs)
							if !ok {
								return false, operatorError("<=", lhs, rhs)
							}
							return res.IsTruthy(), nil
						}
					case ast.GtOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							if lhs.pointer == f64Type && rhs.pointer == f64Type {
								return math.Float64frombits(lhs.scalar) > math.Float64frombits(rhs.scalar), nil
							}
							res, ok := lhs.GreaterThan(rhs)
							if !ok {
								return false, operatorError(">", lhs, rhs)
							}
							return res.IsTruthy(), nil
						}
					case ast.GtEqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							if lhs.pointer == f64Type && rhs.pointer == f64Type {
								return math.Float64frombits(lhs.scalar) >= math.Float64frombits(rhs.scalar), nil
							}
							res, ok := lhs.GreaterThanOrEqualTo(rhs)
							if !ok {
								return false, operatorError(">=", lhs, rhs)
							}
							return res.IsTruthy(), nil
						}
					case ast.EqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							return lhs.Equals(rhs), nil
						}
					case ast.NotEqOp:
						return func(fbr *Fiber) (bool, Exception) {
							lhs, rhs := *fbr.get(lhsLocal), *fbr.get(rhsLocal)
							return !lhs.Equals(rhs), nil
						}
					}
				}
			}
		}
	}

	condition := vm.compile(node)
	return func(fbr *Fiber) (bool, Exception) {
		v, err := condition(fbr)
		if err != nil {
			return false, err
		}
		return v.IsTruthy(), nil
	}
}

func (vm *Instance) emitConditional(node ast.Conditional) instruction {
	condition := vm.compileCondition(node.Condition)
	action := vm.compile(node.Action)

	if node.Otherwise != nil {
		otherwise := vm.compile(node.Otherwise)

		return func(fbr *Fiber) (Value, Exception) {
			ok, err := condition(fbr)
			if err != nil {
				return Value{}, err
			}

			if ok {
				return action(fbr)
			}
			return otherwise(fbr)
		}
	}

	return func(fbr *Fiber) (Value, Exception) {
		ok, err := condition(fbr)
		if err != nil {
			return Value{}, err
		}

		if ok {
			return action(fbr)
		}
		return Value{}, nil
	}
}

func (vm *Instance) emitWhile(node ast.While) instruction {
	condition := vm.compileCondition(node.Condition)
	action := vm.compile(node.Action)

	return func(fbr *Fiber) (Value, Exception) {
		for {
			// evaluate condition
			ok, err := condition(fbr)
			if err != nil {
				return Value{}, err
			}

			if !ok {
				break
			}

			// evaluate action
			v, err := action(fbr)
			if err != nil {
				if err == signalContinue {
					continue
				} else if err == signalBreak {
					break
				}
				return v, err
			}
		}
		return Value{}, nil
	}
}

func (vm *Instance) emitFor(node ast.For) instruction {
	collInst := vm.compile(node.Collection)

	if vm.cp.closures.Len() == 0 {
		vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: &funcInfoStatic{name: "main"}})
		vm.cp.closures.Last(0).scope.OpenBlock()
	}

	vm.cp.closures.Last(0).scope.OpenBlock()
	defer vm.cp.closures.Last(0).scope.CloseBlock()

	var idxIndex int = -1
	if node.IndexVar != "" {
		idx, ok := vm.cp.closures.Last(0).scope.Declare(node.IndexVar, false)
		if !ok {
			panic(CompileErrorf("double declaration of %s in for loop", node.IndexVar))
		}
		idxIndex = idx
	}

	itemIndex, ok := vm.cp.closures.Last(0).scope.Declare(node.ItemVar, false)
	if !ok {
		panic(CompileErrorf("double declaration of %s in for loop", node.ItemVar))
	}

	bodyInst := vm.compile(node.Action)

	return func(fbr *Fiber) (Value, Exception) {
		collVal, err := collInst(fbr)
		if err != nil {
			return collVal, err
		}

		if arr, ok := collVal.AsArray(); ok {
			for i, item := range arr.Data {
				if idxIndex >= 0 {
					fbr.setLocal(idxIndex, BoxNumber(float64(i)))
				}
				fbr.setLocal(itemIndex, item)

				v, err := bodyInst(fbr)
				if err != nil {
					if err == signalContinue {
						continue
					} else if err == signalBreak {
						break
					}
					return v, err
				}
			}
			return Value{}, nil
		}

		if obj, ok := collVal.AsObject(); ok {
			keys := make([]string, 0, len(obj))
			for k := range obj {
				keys = append(keys, fields.Lookup(k))
			}
			sort.Strings(keys)

			for _, k := range keys {
				keyVal := BoxString(k)
				if idxIndex >= 0 {
					fbr.setLocal(idxIndex, keyVal)
					fbr.setLocal(itemIndex, obj[fields.Get(k)])
				} else {
					fbr.setLocal(itemIndex, keyVal)
				}

				v, err := bodyInst(fbr)
				if err != nil {
					if err == signalContinue {
						continue
					} else if err == signalBreak {
						break
					}
					return v, err
				}
			}
			return Value{}, nil
		}

		if str, ok := collVal.AsString(); ok {
			i := 0
			for _, r := range str {
				if idxIndex >= 0 {
					fbr.setLocal(idxIndex, BoxNumber(float64(i)))
				}
				fbr.setLocal(itemIndex, BoxString(string(r)))
				i++

				v, err := bodyInst(fbr)
				if err != nil {
					if err == signalContinue {
						continue
					} else if err == signalBreak {
						break
					}
					return v, err
				}
			}
			return Value{}, nil
		}

		if buf, ok := collVal.AsBuffer(); ok {
			for i, b := range buf {
				if idxIndex >= 0 {
					fbr.setLocal(idxIndex, BoxNumber(float64(i)))
				}
				fbr.setLocal(itemIndex, BoxNumber(float64(b)))

				v, err := bodyInst(fbr)
				if err != nil {
					if err == signalContinue {
						continue
					} else if err == signalBreak {
						break
					}
					return v, err
				}
			}
			return Value{}, nil
		}

		return Value{}, TypeErrorF("cannot iterate over %s", collVal.TypeOf())
	}
}

func (vm *Instance) emitBlock(node ast.Block) instruction {
	if vm.cp.closures.Len() == 0 {
		vm.cp.closures.Push(&closure{freeVars: ds.Set[int]{}, info: &funcInfoStatic{name: "anonymous"}})
		vm.cp.closures.Last(0).scope.OpenBlock()
	}

	vm.cp.closures.Last(0).scope.OpenBlock()
	defer vm.cp.closures.Last(0).scope.CloseBlock()

	// optimise: statement extraction from block; saves an extra dispatch
	if len(node.Code) == 1 && vm.cp.inline {
		node := node.Code[0]
		// optimise: {return x}
		if ret, isReturn := node.(ast.Return); isReturn {
			// optimise: returning constants
			if in, isInput := ret.Value.(ast.Input[float64]); isInput {
				value := BoxNumber(in.Value)
				return func(fbr *Fiber) (Value, Exception) {
					return value, signalReturn
				}
			}

			// optimise: returning locals
			if iGet, isIdentGet := ret.Value.(ast.Ident); isIdentGet {
				variable, err := vm.cp.reach(iGet.Name)
				if err != nil {
					panic(err)
				}

				if v, isLocal := variable.(local); isLocal {
					if v.isCaptured {
						return func(fbr *Fiber) (Value, Exception) {
							return fbr.getCaptured(v.index), signalReturn
						}
					}
					return func(fbr *Fiber) (Value, Exception) {
						return fbr.GetLocal(v.index), signalReturn
					}
				}
			}

			what := vm.compile(ret.Value)
			return func(fbr *Fiber) (Value, Exception) {
				v, err := what(fbr)
				if err != nil {
					return v, err
				}

				return v, signalReturn
			}
		}

		// generic
		return vm.compile(node)
	}

	block := make([]instruction, len(node.Code))
	for i, statement := range node.Code {
		block[i] = vm.compile(statement)
	}

	switch len(block) {
	case 0:
		return func(fbr *Fiber) (Value, Exception) {
			return Value{}, nil
		}
	case 1:
		s0 := block[0]
		return func(fbr *Fiber) (Value, Exception) {
			if v, err := s0(fbr); err != nil {
				return v, err
			}
			return Value{}, nil
		}
	case 2:
		s0 := block[0]
		s1 := block[1]
		return func(fbr *Fiber) (Value, Exception) {
			if v, err := s0(fbr); err != nil {
				return v, err
			}
			if v, err := s1(fbr); err != nil {
				return v, err
			}
			return Value{}, nil
		}
	case 3:
		s0 := block[0]
		s1 := block[1]
		s2 := block[2]
		return func(fbr *Fiber) (Value, Exception) {
			if v, err := s0(fbr); err != nil {
				return v, err
			}
			if v, err := s1(fbr); err != nil {
				return v, err
			}
			if v, err := s2(fbr); err != nil {
				return v, err
			}
			return Value{}, nil
		}
	default:
		return func(fbr *Fiber) (Value, Exception) {
			for _, statement := range block {
				if v, err := statement(fbr); err != nil {
					return v, err
				}
			}
			return Value{}, nil
		}
	}
}
