package ast

import (
	"github.com/hk-32/evie/core"
)

type Fn struct {
	Name   string
	Args   []string
	Action Node
}

type Go struct {
	Routine Node
}

type Call struct {
	Fn   Node
	Args []Node
}

type Return struct {
	Value Node
}

func (fn Fn) compile(vm *Machine) core.Instruction {
	if fn.Name != "" {
		panic("named functions are only allowed as top level declarations")
	}

	vm.openFunction()
	vm.scopeExtend()

	// declare the fn arguments and only then compile the code
	for _, arg := range fn.Args {
		vm.declare(arg)
	}
	action := fn.Action.compile(vm)

	capacity := vm.scopeCapacity()
	refs, escapees := vm.closeFunction()
	vm.scopeDeExtend()

	// make list of non-escaping variables so they can be freed after execution
	freeable := make([]int, 0, capacity-len(escapees))
	for index := range capacity {
		if _, exists := escapees[index]; !exists {
			freeable = append(freeable, index)
		}
	}

	info := &core.FuncInfoStatic{
		Name:        "λ",
		Args:        fn.Args,
		Refs:        refs,
		NonEscaping: freeable,
		Capacity:    capacity,
		Code:        action,
		Machine:     &vm.Machine}

	return func(rt *core.CoRoutine) (core.Value, error) {
		captured := make([]*core.Value, len(refs))
		for i, ref := range refs {
			captured[i] = rt.Capture(ref.Index, ref.Scroll)
		}

		// create the user fn & return it
		return core.BoxUserFn(core.UserFn{Captured: captured, FuncInfoStatic: info}), nil
	}
}

func (fn Fn) compileInGlobal(vm *Machine) core.Instruction {
	index := vm.get(fn.Name)

	vm.openFunction()
	vm.scopeExtend()

	// declare the fn arguments and only then compile the code
	for _, arg := range fn.Args {
		vm.declare(arg)
	}
	action := fn.Action.compile(vm)

	capacity := vm.scopeCapacity()
	refs, escapees := vm.closeFunction()
	vm.scopeDeExtend()

	// make list of non-escaping variables so they can be freed after execution
	freeable := make([]int, 0, capacity-len(escapees))
	for index := range capacity {
		if _, exists := escapees[index]; !exists {
			freeable = append(freeable, index)
		}
	}

	info := &core.FuncInfoStatic{
		Name:        fn.Name,
		Args:        fn.Args,
		Refs:        refs,
		NonEscaping: freeable,
		Capacity:    capacity,
		Code:        action,
		Machine:     &vm.Machine}

	return func(rt *core.CoRoutine) (core.Value, error) {
		captured := make([]*core.Value, len(refs))
		for i, ref := range refs {
			captured[i] = rt.Capture(ref.Index, ref.Scroll)
		}

		// create the user fn
		fn := core.BoxUserFn(core.UserFn{Captured: captured, FuncInfoStatic: info})

		// declare the function locally
		rt.StoreLocal(index, fn)
		return core.Value{}, nil
	}
}

func (call Call) compile(vm *Machine) core.Instruction {
	// compile arguments
	argsFetchers := make([]core.Instruction, len(call.Args))
	for i, arg := range call.Args {
		argsFetchers[i] = arg.compile(vm)
	}

	// optimise: calling captured functions
	if iGet, isIdentGet := call.Fn.(IdentGet); isIdentGet && vm.optimise {
		ref := vm.reach(iGet.Name)
		if ref.IsCaptured() {
			index := vm.addToCaptured(ref)
			return func(rt *core.CoRoutine) (core.Value, error) {
				value := rt.GetCaptured(index)

				// check if its a user function
				if fn, isUserFn := value.AsUserFn(); isUserFn {
					if len(fn.Args) != len(argsFetchers) {
						if fn.Name != "λ" {
							return core.Value{}, core.CustomError("function '%v' requires %v argument(s), %v provided", fn.Name, len(fn.Args), len(argsFetchers))
						}
						return core.Value{}, core.CustomError("function requires %v argument(s), %v provided", len(fn.Args), len(argsFetchers))
					}

					// create space for all the locals
					base := len(rt.Stack)
					for range fn.Capacity {
						rt.Stack = append(rt.Stack, vm.Boxes.New())
					}

					// set arguments
					for i, fetcher := range argsFetchers {
						v, err := fetcher(rt)
						if err != nil {
							return v, err
						}
						*rt.Stack[base+i] = v
					}
					rt.PushBase(base)

					// save old state
					oldCaptured := rt.Captured

					rt.Captured = fn.Captured
					// run fn code
					result, err := fn.Code(rt)

					// release non-escaping locals
					for _, index := range fn.NonEscaping {
						vm.Boxes.Put(rt.Stack[base+index])
					}

					// restore old state
					rt.ExitUserFN(fn.Capacity, oldCaptured)

					// don't implicitly return the return value of the last executed instruction
					switch err {
					case nil:
						return core.Value{}, nil
					case core.ErrReturnSignal:
						return result, nil
					default:
						return result, err
					}
				}

				return core.Value{}, core.CustomError("cannot call a non-function '%v'", value)
			}
		}
	}

	// compile function fetcher
	fnFetcher := call.Fn.compile(vm)
	return func(rt *core.CoRoutine) (core.Value, error) {
		value, err := fnFetcher(rt)
		if err != nil {
			return value, err
		}

		// check if its a user function
		if fn, isUserFn := value.AsUserFn(); isUserFn {
			if len(fn.Args) != len(argsFetchers) {
				if fn.Name != "λ" {
					return core.Value{}, core.CustomError("function '%v' requires %v argument(s), %v provided", fn.Name, len(fn.Args), len(argsFetchers))
				}
				return core.Value{}, core.CustomError("function requires %v argument(s), %v provided", len(fn.Args), len(argsFetchers))
			}

			// create space for all the locals
			base := len(rt.Stack)
			for range fn.Capacity {
				rt.Stack = append(rt.Stack, vm.Boxes.New())
			}

			// set arguments
			for i, fetcher := range argsFetchers {
				v, err := fetcher(rt)
				if err != nil {
					return v, err
				}
				*rt.Stack[base+i] = v
			}
			rt.PushBase(base)

			// save old state
			oldCaptured := rt.Captured

			rt.Captured = fn.Captured
			// run fn code
			value, err = fn.Code(rt)

			// release non-escaping locals
			for _, index := range fn.NonEscaping {
				vm.Boxes.Put(rt.Stack[base+index])
			}

			// restore old state
			rt.ExitUserFN(fn.Capacity, oldCaptured)

			// don't implicitly return the return value of the last executed instruction
			switch err {
			case nil:
				return core.Value{}, nil
			case core.ErrReturnSignal:
				return value, nil
			default:
				return value, err
			}
		}

		return core.Value{}, core.CustomError("cannot call a non-function '%v'", value)
	}
}

func (g Go) compile(vm *Machine) core.Instruction {
	/* if call, isCall := g.Routine.(Call); isCall {
		pos := vm.emit(op.GO, byte(len(call.Args)))
		call.Fn.compile(vm)
		for _, arg := range call.Args {
			arg.compile(vm)
		}
		return pos
	} else if call, isCall := g.Routine.(DotCall); isCall {
		pos := call.compile2(vm)
		vm.set(pos, op.GO)
		return pos
	}

	panic("go expected call, got something else") */

	return func(rt *core.CoRoutine) (core.Value, error) {
		return core.Value{}, nil
	}
}

func (ret Return) compile(vm *Machine) core.Instruction {
	// optimise: returning contants
	if in, isInput := ret.Value.(Input); isInput && vm.optimise {
		return func(rt *core.CoRoutine) (core.Value, error) {
			return in.Value, core.ErrReturnSignal
		}
	}

	// optimise: returning local variables
	if iGet, isIdentGet := ret.Value.(IdentGet); isIdentGet && vm.optimise {
		ref := vm.reach(iGet.Name)
		if ref.IsLocal() {
			return func(rt *core.CoRoutine) (core.Value, error) {
				return rt.GetLocal(ref.Index), core.ErrReturnSignal
			}
		}
	}

	what := ret.Value.compile(vm)
	return func(rt *core.CoRoutine) (core.Value, error) {
		v, err := what(rt)
		if err != nil {
			return v, err
		}

		return v, core.ErrReturnSignal
	}
}
