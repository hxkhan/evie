package vm

import (
	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/ds"
)

func (vm *Instance) emitReturn(node ast.Return) instruction {
	if vm.cp.inline {
		// optimise: returning constants
		if in, isInput := node.Value.(ast.Input[float64]); isInput {
			value := BoxNumber(in.Value)
			return func(fbr *fiber) (Value, exception) {
				return value, signalReturn
			}
		}

		// optimise: returning locals variable
		if iGet, isIdentGet := node.Value.(ast.Ident); isIdentGet {
			variable, err := vm.cp.reach(iGet.Name)
			if err != nil {
				panic(vm.locate(err, iGet))
			}

			if v, isLocal := variable.(local); isLocal {
				if v.isCaptured {
					return func(fbr *fiber) (Value, exception) {
						return fbr.getCaptured(v.index), signalReturn
					}
				}
				return func(fbr *fiber) (Value, exception) {
					return fbr.getLocal(v.index), signalReturn
				}
			}
		}
	}

	what := vm.compile(node.Value)
	return func(fbr *fiber) (Value, exception) {
		v, err := what(fbr)
		if err != nil {
			return v, err
		}

		return v, signalReturn
	}
}

func (vm *Instance) emitThrow(node ast.Throw) instruction {
	site := vm.at(node)
	what := vm.compile(node.Value)
	return func(fbr *fiber) (Value, exception) {
		v, err := what(fbr)
		if err != nil {
			return v, err
		}

		if e, ok := v.AsError(); ok {
			return Value{}, e.at(site)
		}

		return Value{}, errorAt(site, "%s", v.String())
	}
}

// compileCondition compiles an expression used as a condition straight to a
// Go bool; comparisons on locals and constants skip the boxed result
func (vm *Instance) compileCondition(node ast.Node) func(fbr *fiber) (bool, exception) {
	site := vm.at(node)
	if val, ok := vm.evaluate(node).(Value); ok {
		b := val.IsTruthy()
		return func(fbr *fiber) (bool, exception) {
			return b, nil
		}
	}

	if bin, ok := node.(ast.BinOp); ok {
		if fast, ok := vm.compileComparison(bin, site); ok {
			return fast
		}
	}

	condition := vm.compile(node)
	return func(fbr *fiber) (bool, exception) {
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

		return func(fbr *fiber) (Value, exception) {
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

	return func(fbr *fiber) (Value, exception) {
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
	site := vm.at(node)
	condition := vm.compileCondition(node.Condition)
	action := vm.compile(node.Action)

	return func(fbr *fiber) (Value, exception) {
		for {
			if fbr.signaled() {
				if exc := fbr.halt(site); exc != nil {
					return Value{}, exc
				}
			}
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
	site := vm.at(node)
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
			panic(vm.compileErrorAt(node, "double declaration of %s in for loop", node.IndexVar))
		}
		idxIndex = idx
	}

	itemIndex, ok := vm.cp.closures.Last(0).scope.Declare(node.ItemVar, false)
	if !ok {
		panic(vm.compileErrorAt(node, "double declaration of %s in for loop", node.ItemVar))
	}

	bodyInst := vm.compile(node.Action)

	return func(fbr *fiber) (Value, exception) {
		collVal, err := collInst(fbr)
		if err != nil {
			return collVal, err
		}

		if arr, ok := collVal.AsArray(); ok {
			for i, item := range arr.Data {
				if fbr.signaled() {
					if exc := fbr.halt(site); exc != nil {
						return Value{}, exc
					}
				}
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
			for _, k := range sortedKeys(obj) {
				if fbr.signaled() {
					if exc := fbr.halt(site); exc != nil {
						return Value{}, exc
					}
				}
				keyVal := BoxString(k)
				if idxIndex >= 0 {
					fbr.setLocal(idxIndex, keyVal)
					fbr.setLocal(itemIndex, obj[k])
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
				if fbr.signaled() {
					if exc := fbr.halt(site); exc != nil {
						return Value{}, exc
					}
				}
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
				if fbr.signaled() {
					if exc := fbr.halt(site); exc != nil {
						return Value{}, exc
					}
				}
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

		return Value{}, errorAt(site, "cannot iterate over %s", article(collVal)).hinted("for loops run over arrays, objects, strings and buffers")
	}
}

type switchCaseCompiled struct {
	values []instruction
	action instruction
}

func (vm *Instance) emitSwitch(node ast.Switch) instruction {
	cases := make([]switchCaseCompiled, len(node.Cases))
	for i, c := range node.Cases {
		vals := make([]instruction, len(c.Values))
		for j, val := range c.Values {
			vals[j] = vm.compile(val)
		}
		cases[i] = switchCaseCompiled{
			values: vals,
			action: vm.compile(c.Action),
		}
	}

	var defaultAction instruction
	if node.Default != nil {
		defaultAction = vm.compile(node.Default)
	}

	// Tagless switch: switch { case cond1, cond2: ... }
	if node.Value == nil {
		return func(fbr *fiber) (Value, exception) {
			for _, c := range cases {
				matched := false
				for _, valInst := range c.values {
					condVal, err := valInst(fbr)
					if err != nil {
						return condVal, err
					}
					if condVal.IsTruthy() {
						matched = true
						break
					}
				}
				if matched {
					v, err := c.action(fbr)
					if err == signalBreak {
						return Value{}, nil
					}
					return v, err
				}
			}
			if defaultAction != nil {
				v, err := defaultAction(fbr)
				if err == signalBreak {
					return Value{}, nil
				}
				return v, err
			}
			return Value{}, nil
		}
	}

	// Value switch: switch expr { case val1, val2: ... }
	targetInst := vm.compile(node.Value)
	return func(fbr *fiber) (Value, exception) {
		targetVal, err := targetInst(fbr)
		if err != nil {
			return targetVal, err
		}

		for _, c := range cases {
			matched := false
			for _, valInst := range c.values {
				caseVal, err := valInst(fbr)
				if err != nil {
					return caseVal, err
				}
				if targetVal.Equals(caseVal) {
					matched = true
					break
				}
			}
			if matched {
				v, err := c.action(fbr)
				if err == signalBreak {
					return Value{}, nil
				}
				return v, err
			}
		}

		if defaultAction != nil {
			v, err := defaultAction(fbr)
			if err == signalBreak {
				return Value{}, nil
			}
			return v, err
		}
		return Value{}, nil
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
				return func(fbr *fiber) (Value, exception) {
					return value, signalReturn
				}
			}

			// optimise: returning locals
			if iGet, isIdentGet := ret.Value.(ast.Ident); isIdentGet {
				variable, err := vm.cp.reach(iGet.Name)
				if err != nil {
					panic(vm.locate(err, iGet))
				}

				if v, isLocal := variable.(local); isLocal {
					if v.isCaptured {
						return func(fbr *fiber) (Value, exception) {
							return fbr.getCaptured(v.index), signalReturn
						}
					}
					return func(fbr *fiber) (Value, exception) {
						return fbr.getLocal(v.index), signalReturn
					}
				}
			}

			what := vm.compile(ret.Value)
			return func(fbr *fiber) (Value, exception) {
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
		return func(fbr *fiber) (Value, exception) {
			return Value{}, nil
		}
	case 1:
		s0 := block[0]
		return func(fbr *fiber) (Value, exception) {
			if v, err := s0(fbr); err != nil {
				return v, err
			}
			return Value{}, nil
		}
	case 2:
		s0 := block[0]
		s1 := block[1]
		return func(fbr *fiber) (Value, exception) {
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
		return func(fbr *fiber) (Value, exception) {
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
		return func(fbr *fiber) (Value, exception) {
			for _, statement := range block {
				if v, err := statement(fbr); err != nil {
					return v, err
				}
			}
			return Value{}, nil
		}
	}
}
