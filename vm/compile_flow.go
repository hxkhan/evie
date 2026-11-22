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
				if v.isBoxed {
					return func(fbr *fiber) (Value, exception) {
						return fbr.getBoxedLocal(v.index), signalReturn
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
			if e.fatal || e.Pos.Known() {
				// thrown before: the script may still hold it, and a stop it caught
				// as a value is an ordinary error when thrown again
				e = e.clone(false)
			}
			return Value{}, e.at(site)
		}

		return Value{}, errorAt(site, "%s", v.String())
	}
}

// compileCondition compiles an expression used as a condition straight to a
// Go bool; comparisons on locals and constants skip the boxed result, and &&,
// || and ! combine their operands' conditions without boxing either
func (vm *Instance) compileCondition(node ast.Node) func(fbr *fiber) (bool, exception) {
	site := vm.at(node)
	if val, ok := vm.evaluate(node).(Value); ok {
		b := val.IsTruthy()
		return func(fbr *fiber) (bool, exception) {
			return b, nil
		}
	}

	switch n := node.(type) {
	case ast.BinOp:
		switch n.Operator {
		case ast.AndOp:
			lhs, rhs := vm.compileCondition(n.Lhs), vm.compileCondition(n.Rhs)
			return func(fbr *fiber) (bool, exception) {
				if ok, err := lhs(fbr); !ok || err != nil {
					return false, err
				}
				return rhs(fbr)
			}
		case ast.OrOp:
			lhs, rhs := vm.compileCondition(n.Lhs), vm.compileCondition(n.Rhs)
			return func(fbr *fiber) (bool, exception) {
				if ok, err := lhs(fbr); ok || err != nil {
					return ok, err
				}
				return rhs(fbr)
			}
		}
		if fast, ok := vm.compileComparison(n, site); ok {
			return fast
		}
	case ast.Not:
		inner := vm.compileCondition(n.Value)
		return func(fbr *fiber) (bool, exception) {
			ok, err := inner(fbr)
			return !ok, err
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
	var otherwise instruction
	if node.Otherwise != nil {
		otherwise = vm.compile(node.Otherwise)
	}

	// a condition known while compiling runs one branch; both are compiled, so
	// mistakes in the other are still reported
	if val, ok := vm.evaluate(node.Condition).(Value); ok {
		switch {
		case val.IsTruthy():
			return action
		case otherwise != nil:
			return otherwise
		}
		return func(fbr *fiber) (Value, exception) { return Value{}, nil }
	}

	if otherwise != nil {

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

// containsContinue reports whether any statement in stmts executes a continue
// targeting the immediately enclosing loop (skipping nested loops).
func containsContinue(stmts []ast.Node) bool {
	found := false
	for _, stmt := range stmts {
		ast.Walk(stmt, func(n ast.Node) bool {
			if found {
				return false
			}
			switch n.(type) {
			case ast.Continue:
				found = true
				return false
			case ast.While, ast.For, ast.Fn, ast.ClassDefinition:
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

// emitCountedWhile fuses common numeric while loops (`while i < limit { ...; i += step }`)
// directly into one Go loop without per-iteration condition, block, or step
// closure calls. Returns nil when the loop does not match.
func (vm *Instance) emitCountedWhile(node ast.While, site site) instruction {
	if !vm.cp.inline {
		return nil
	}
	bin, ok := node.Condition.(ast.BinOp)
	if !ok || (bin.Operator != ast.LtOp && bin.Operator != ast.LtEqOp) {
		return nil
	}
	idxIdent, ok := bin.Lhs.(ast.Ident)
	if !ok {
		return nil
	}
	idxLocal, ok := vm.evaluate(idxIdent).(local)
	if !ok || idxLocal.isCaptured || idxLocal.isBoxed {
		return nil
	}

	var limitLocal local
	var limitConst float64
	limitIsLocal := false
	switch r := vm.evaluate(bin.Rhs).(type) {
	case local:
		if r.isCaptured || r.isBoxed {
			return nil
		}
		limitLocal = r
		limitIsLocal = true
	case Value:
		if r.pointer != f64Type {
			return nil
		}
		limitConst = math.Float64frombits(r.scalar)
	default:
		return nil
	}

	block, ok := node.Action.(ast.Block)
	if !ok || len(block.Code) == 0 {
		return nil
	}

	// Check if the last statement is `i += step` where `i` matches the loop variable,
	// nothing in the block shadows `i` (or `step`), and none of the preceding
	// statements `continue` (which would skip the step).
	// The step is analysed before the body is compiled, outside the body's scope, so it
	// may only name bindings from before the loop: a name the body declares would
	// resolve to an outer binding or to nothing.
	declaresName := func(stmts []ast.Node, name string) bool {
		for _, stmt := range stmts {
			switch d := stmt.(type) {
			case ast.Decl:
				if d.Name == name {
					return true
				}
			case ast.Fn:
				if !d.UsedAsExpr && d.Name == name {
					return true
				}
			case ast.ClassDefinition:
				if d.Name == name {
					return true
				}
			}
		}
		return false
	}
	namesAny := func(n ast.Node, stmts []ast.Node) (found bool) {
		ast.Walk(n, func(sub ast.Node) bool {
			if id, ok := sub.(ast.Ident); ok && declaresName(stmts, id.Name) {
				found = true
			}
			return !found
		})
		return found
	}
	hasStep := false
	stepIsLocal := false
	var stepLocal local
	var stepConst float64
	bodyNodes := block.Code
	if mbo, ok := block.Code[len(block.Code)-1].(ast.MutableBinOp); ok && mbo.Operator == ast.AddOp {
		prefix := block.Code[:len(block.Code)-1]
		if lhsIdent, ok := mbo.Lhs.(ast.Ident); ok && lhsIdent.Name == idxIdent.Name && !containsContinue(prefix) && !declaresName(prefix, idxIdent.Name) && !namesAny(mbo.Rhs, prefix) {
			if target, ok := vm.evaluate(lhsIdent).(local); ok && target == idxLocal && !target.isStatic {
				switch r := vm.evaluate(mbo.Rhs).(type) {
				case Value:
					if r.pointer == f64Type {
						hasStep = true
						stepConst = math.Float64frombits(r.scalar)
						bodyNodes = prefix
					}
				case local:
					if _, ok := mbo.Rhs.(ast.Ident); ok && !r.isCaptured && !r.isBoxed {
						hasStep = true
						stepIsLocal = true
						stepLocal = r
						bodyNodes = prefix
					}
				}
			}
		}
	}

	cl := vm.currentClosure()
	cl.scope.OpenBlock()
	cl.loops++
	stmts := make([]instruction, len(bodyNodes))
	for i, s := range bodyNodes {
		stmts[i] = vm.compile(s)
	}
	cl.loops--
	cl.scope.CloseBlock()

	// Fallback instructions only used if a runtime value stops being a number.
	fallbackCond := vm.compileCondition(node.Condition)
	var fallbackStep instruction
	if hasStep {
		fallbackStep = vm.compile(block.Code[len(block.Code)-1])
	}

	ii := int(idxLocal.index)
	li := int(limitLocal.index)
	si := int(stepLocal.index)
	isLt := bin.Operator == ast.LtOp

	// Specialize the ultra-hot 1-statement body with fused step (`sieve`, `matmul`, `objects`).
	if hasStep && len(stmts) == 1 && isLt {
		s0 := stmts[0]
		if limitIsLocal && !stepIsLocal {
			return func(fbr *fiber) (Value, exception) {
				for {
					if fbr.signaled() {
						if exc := fbr.halt(site); exc != nil {
							return Value{}, exc
						}
					}
					s := fbr.stack[fbr.base:]
					iv, lv := s[ii], s[li]
					if iv.pointer == f64Type && lv.pointer == f64Type {
						if !(math.Float64frombits(iv.scalar) < math.Float64frombits(lv.scalar)) {
							break
						}
					} else {
						ok, err := fallbackCond(fbr)
						if err != nil {
							return Value{}, err
						}
						if !ok {
							break
						}
					}
					if v, err := s0(fbr); err != nil {
						if err == signalBreak {
							break
						}
						return v, err
					}
					iv = fbr.stack[fbr.base+ii]
					if iv.pointer == f64Type {
						fbr.stack[fbr.base+ii] = Value{pointer: f64Type, scalar: math.Float64bits(math.Float64frombits(iv.scalar) + stepConst)}
					} else if v, err := fallbackStep(fbr); err != nil {
						return v, err
					}
				}
				return Value{}, nil
			}
		}
		if limitIsLocal && stepIsLocal {
			return func(fbr *fiber) (Value, exception) {
				for {
					if fbr.signaled() {
						if exc := fbr.halt(site); exc != nil {
							return Value{}, exc
						}
					}
					s := fbr.stack[fbr.base:]
					iv, lv := s[ii], s[li]
					if iv.pointer == f64Type && lv.pointer == f64Type {
						if !(math.Float64frombits(iv.scalar) < math.Float64frombits(lv.scalar)) {
							break
						}
					} else {
						ok, err := fallbackCond(fbr)
						if err != nil {
							return Value{}, err
						}
						if !ok {
							break
						}
					}
					if v, err := s0(fbr); err != nil {
						if err == signalBreak {
							break
						}
						return v, err
					}
					s = fbr.stack[fbr.base:]
					iv, sv := s[ii], s[si]
					if iv.pointer == f64Type && sv.pointer == f64Type {
						s[ii] = Value{pointer: f64Type, scalar: math.Float64bits(math.Float64frombits(iv.scalar) + math.Float64frombits(sv.scalar))}
					} else if v, err := fallbackStep(fbr); err != nil {
						return v, err
					}
				}
				return Value{}, nil
			}
		}
		if !limitIsLocal && !stepIsLocal {
			return func(fbr *fiber) (Value, exception) {
				for {
					if fbr.signaled() {
						if exc := fbr.halt(site); exc != nil {
							return Value{}, exc
						}
					}
					iv := fbr.stack[fbr.base+ii]
					if iv.pointer == f64Type {
						if !(math.Float64frombits(iv.scalar) < limitConst) {
							break
						}
					} else {
						ok, err := fallbackCond(fbr)
						if err != nil {
							return Value{}, err
						}
						if !ok {
							break
						}
					}
					if v, err := s0(fbr); err != nil {
						if err == signalBreak {
							break
						}
						return v, err
					}
					iv = fbr.stack[fbr.base+ii]
					if iv.pointer == f64Type {
						fbr.stack[fbr.base+ii] = Value{pointer: f64Type, scalar: math.Float64bits(math.Float64frombits(iv.scalar) + stepConst)}
					} else if v, err := fallbackStep(fbr); err != nil {
						return v, err
					}
				}
				return Value{}, nil
			}
		}
	}

	return func(fbr *fiber) (Value, exception) {
		for {
			if fbr.signaled() {
				if exc := fbr.halt(site); exc != nil {
					return Value{}, exc
				}
			}
			s := fbr.stack[fbr.base:]
			iv := s[ii]
			if limitIsLocal {
				lv := s[li]
				if iv.pointer == f64Type && lv.pointer == f64Type {
					a, b := math.Float64frombits(iv.scalar), math.Float64frombits(lv.scalar)
					if isLt {
						if !(a < b) {
							break
						}
					} else {
						if !(a <= b) {
							break
						}
					}
				} else {
					ok, err := fallbackCond(fbr)
					if err != nil {
						return Value{}, err
					}
					if !ok {
						break
					}
				}
			} else {
				if iv.pointer == f64Type {
					a := math.Float64frombits(iv.scalar)
					if isLt {
						if !(a < limitConst) {
							break
						}
					} else {
						if !(a <= limitConst) {
							break
						}
					}
				} else {
					ok, err := fallbackCond(fbr)
					if err != nil {
						return Value{}, err
					}
					if !ok {
						break
					}
				}
			}

			broke := false
			continued := false
			for _, stmt := range stmts {
				if v, err := stmt(fbr); err != nil {
					if err == signalContinue {
						continued = true
						break
					}
					if err == signalBreak {
						broke = true
						break
					}
					return v, err
				}
			}
			if broke {
				break
			}
			if continued {
				continue
			}
			if hasStep {
				s = fbr.stack[fbr.base:]
				iv = s[ii]
				if !stepIsLocal {
					if iv.pointer == f64Type {
						s[ii] = Value{pointer: f64Type, scalar: math.Float64bits(math.Float64frombits(iv.scalar) + stepConst)}
					} else if v, err := fallbackStep(fbr); err != nil {
						return v, err
					}
				} else {
					sv := s[si]
					if iv.pointer == f64Type && sv.pointer == f64Type {
						s[ii] = Value{pointer: f64Type, scalar: math.Float64bits(math.Float64frombits(iv.scalar) + math.Float64frombits(sv.scalar))}
					} else if v, err := fallbackStep(fbr); err != nil {
						return v, err
					}
				}
			}
		}
		return Value{}, nil
	}
}

func (vm *Instance) emitWhile(node ast.While) instruction {
	site := vm.at(node)
	if fast := vm.emitCountedWhile(node, site); fast != nil {
		return fast
	}
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
			keys := obj.keys // Delete never changes it in place, so the loop survives changes
			for i, k := range keys {
				if fbr.signaled() {
					if exc := fbr.halt(site); exc != nil {
						return Value{}, exc
					}
				}
				keyVal := BoxString(k)
				if idxIndex >= 0 {
					v, ok := obj.at(i, k)
					if !ok {
						continue // removed by an earlier iteration
					}
					bindIdx(fbr, keyVal)
					bindItem(fbr, v)
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
	conds  []func(fbr *fiber) (bool, exception) // a tagless switch's case conditions
	action instruction
}

func (vm *Instance) emitSwitch(node ast.Switch) instruction {
	cl := vm.currentClosure()

	cases := make([]switchCaseCompiled, len(node.Cases))
	for i, c := range node.Cases {
		var vals []instruction
		var conds []func(fbr *fiber) (bool, exception)
		for _, val := range c.Values {
			if node.Value == nil {
				conds = append(conds, vm.compileCondition(val))
			} else {
				vals = append(vals, vm.compile(val))
			}
		}
		cl.switches++
		action := vm.compile(c.Action)
		cl.switches--
		cases[i] = switchCaseCompiled{
			values: vals,
			conds:  conds,
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
				for _, cond := range c.conds {
					ok, err := cond(fbr)
					if err != nil {
						return Value{}, err
					}
					if ok {
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
