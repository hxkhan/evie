package vm

import (
	"math"

	"github.com/hxkhan/evie/ast"
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
		if vm.cp.inline && (bin.Operator == ast.EqOp || bin.Operator == ast.NotEqOp) {
			if rhs, ok := vm.evaluate(bin.Rhs).(Value); ok && rhs.pointer == f64Type {
				lhs := vm.compile(bin.Lhs)
				rn := math.Float64frombits(rhs.scalar)
				if bin.Operator == ast.EqOp {
					return func(fbr *fiber) (bool, exception) {
						a, err := lhs(fbr)
						if err != nil {
							return false, err
						}
						return a.pointer == f64Type && math.Float64frombits(a.scalar) == rn, nil
					}
				}
				return func(fbr *fiber) (bool, exception) {
					a, err := lhs(fbr)
					if err != nil {
						return false, err
					}
					return a.pointer != f64Type || math.Float64frombits(a.scalar) != rn, nil
				}
			}
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

	cl := vm.currentClosure()
	cl.loops++
	action := vm.compile(node.Action)
	cl.loops--

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

	cl := vm.currentClosure()
	cl.scope.OpenBlock()
	defer cl.scope.CloseBlock()

	var idxIndex int = -1
	if node.IndexVar != "" {
		idx, ok := cl.scope.Declare(node.IndexVar, false)
		if !ok {
			panic(vm.compileErrorAt(node, "double declaration of %s in for loop", node.IndexVar))
		}
		idxIndex = idx
	}

	itemIndex, ok := cl.scope.Declare(node.ItemVar, false)
	if !ok {
		panic(vm.compileErrorAt(node, "double declaration of %s in for loop", node.ItemVar))
	}

	cl.loops++
	bodyInst := vm.compile(node.Action)
	cl.loops--

	idxEscapes := idxIndex >= 0 && cl.mayEscape(node.IndexVar)
	itemEscapes := cl.mayEscape(node.ItemVar)

	bindIdx := func(fbr *fiber, v Value) {
		if idxEscapes {
			fbr.setEscapingLocal(idxIndex, v)
		} else {
			fbr.setLocal(idxIndex, v)
		}
	}
	bindItem := func(fbr *fiber, v Value) {
		if itemEscapes {
			fbr.setEscapingLocal(itemIndex, v)
		} else {
			fbr.setLocal(itemIndex, v)
		}
	}

	return func(fbr *fiber) (Value, exception) {
		collVal, err := collInst(fbr)
		if err != nil {
			return collVal, err
		}

		if arr, ok := collVal.AsArray(); ok {
			if !idxEscapes && !itemEscapes {
				if idxIndex >= 0 {
					for i, item := range arr.Data {
						if fbr.signaled() {
							if exc := fbr.halt(site); exc != nil {
								return Value{}, exc
							}
						}
						fbr.setLocal(idxIndex, BoxNumber(float64(i)))
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
				} else {
					for _, item := range arr.Data {
						if fbr.signaled() {
							if exc := fbr.halt(site); exc != nil {
								return Value{}, exc
							}
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
				}
				return Value{}, nil
			}
			for i, item := range arr.Data {
				if fbr.signaled() {
					if exc := fbr.halt(site); exc != nil {
						return Value{}, exc
					}
				}
				if idxIndex >= 0 {
					bindIdx(fbr, BoxNumber(float64(i)))
				}
				bindItem(fbr, item)

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
					bindIdx(fbr, keyVal)
					bindItem(fbr, obj[k])
				} else {
					bindItem(fbr, keyVal)
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
					bindIdx(fbr, BoxNumber(float64(i)))
				}
				bindItem(fbr, BoxString(string(r)))
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
					bindIdx(fbr, BoxNumber(float64(i)))
				}
				bindItem(fbr, BoxNumber(float64(b)))

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
	cl := vm.currentClosure()

	cases := make([]switchCaseCompiled, len(node.Cases))
	for i, c := range node.Cases {
		vals := make([]instruction, len(c.Values))
		for j, val := range c.Values {
			vals[j] = vm.compile(val)
		}
		cl.switches++
		action := vm.compile(c.Action)
		cl.switches--
		cases[i] = switchCaseCompiled{
			values: vals,
			action: action,
		}
	}

	var defaultAction instruction
	if node.Default != nil {
		cl.switches++
		defaultAction = vm.compile(node.Default)
		cl.switches--
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
	cl := vm.currentClosure()
	cl.scope.OpenBlock()
	defer cl.scope.CloseBlock()

	// optimise: a single-statement block whose statement already yields Value{}
	// (or returns directly) can be emitted directly, skipping the block wrapper
	if len(node.Code) == 1 && vm.cp.inline {
		stmt := node.Code[0]
		switch n := stmt.(type) {
		case ast.Return, ast.Decl, ast.Assign, ast.MutableBinOp, ast.Conditional, ast.While, ast.For, ast.Switch, ast.Echo, ast.Break, ast.Continue, ast.Throw, ast.Block, ast.ClassDefinition:
			return vm.compile(stmt)
		case ast.Fn:
			if !n.UsedAsExpr {
				return vm.compile(stmt)
			}
		}
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
