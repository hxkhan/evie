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

func (fn Fn) compile(vm *Machine) {
	vm.openFunction()
	vm.scopeExtend()

	// declare the fn arguments and only then compile the code
	for _, arg := range fn.Args {
		vm.declare(arg)
	}

	init := len(vm.Code)
	vm.emit(nil)
	fn.Action.compile(vm)
	skip := len(vm.Code) - init

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
		Start:       init + 1,
		Size:        skip - 1,
		Machine:     &vm.Machine}

	vm.Code[init] = func(rt *core.CoRoutine) (int, error) {
		captured := make([]*core.Value, len(refs))
		for i, ref := range refs {
			captured[i] = rt.Capture(ref.Index, ref.Scroll)
		}

		// create the user fn & push it to the stack
		fn := core.BoxUserFn(core.UserFn{Captured: captured, FuncInfoStatic: info})
		rt.Stack = append(rt.Stack, fn)
		return skip, nil
	}
}

func (fn Fn) compileInGlobal(vm *Machine) {
	index := vm.get(fn.Name)

	vm.openFunction()
	vm.scopeExtend()

	// declare the fn arguments and only then compile the code
	for _, arg := range fn.Args {
		vm.declare(arg)
	}

	init := len(vm.Code)
	vm.emit(nil)
	fn.Action.compile(vm)
	vm.emitImplicitReturn()
	skip := len(vm.Code) - init

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
		Start:       init + 1,
		Size:        skip - 1,
		Machine:     &vm.Machine}

	vm.Code[init] = func(rt *core.CoRoutine) (int, error) {
		captured := make([]*core.Value, len(refs))
		for i, ref := range refs {
			captured[i] = rt.Capture(ref.Index, ref.Scroll)
		}

		// create the user fn & store it locally
		fn := core.BoxUserFn(core.UserFn{Captured: captured, FuncInfoStatic: info})
		rt.StoreLocal(index, fn)
		return skip, nil
	}
}

func (call Call) compile(vm *Machine) {
	// compile arguments
	for _, arg := range call.Args {
		arg.compile(vm)
	}

	// compile function fetcher
	call.Fn.compile(vm)

	vm.emit(func(rt *core.CoRoutine) (int, error) {
		return runner(rt, vm, len(call.Args))
	})
}

func (g Go) compile(vm *Machine) {
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
	} */

	panic("go expected call, got something else")

}

func (ret Return) compile(vm *Machine) {
	ret.Value.compile(vm)
	vm.emit(func(rt *core.CoRoutine) (int, error) {
		rt.PopFrame(&vm.Machine)
		return 1, nil
	})
}

func (vm *Machine) emitImplicitReturn() {
	vm.emit(NULL)
	vm.emit(func(rt *core.CoRoutine) (int, error) {
		rt.PopFrame(&vm.Machine)
		return 1, nil
	})
}

func NULL(rt *core.CoRoutine) (int, error) {
	rt.Stack = append(rt.Stack, core.Value{})
	return 1, nil
}

func runner(rt *core.CoRoutine, vm *Machine, nargsProvided int) (int, error) {
	value := rt.Stack[len(rt.Stack)-1]
	rt.Stack = rt.Stack[:len(rt.Stack)-1]

	// check if its a user function
	if fn, isUserFn := value.AsUserFn(); isUserFn {
		if len(fn.Args) != nargsProvided {
			if fn.Name != "λ" {
				return 1, core.CustomError("function '%v' requires %v argument(s), %v provided", fn.Name, len(fn.Args), nargsProvided)
			}
			return 1, core.CustomError("function requires %v argument(s), %v provided", len(fn.Args), nargsProvided)
		}

		// create space for all the locals
		base := len(rt.Locals)
		for range fn.Capacity {
			rt.Locals = append(rt.Locals, vm.Boxes.New())
		}

		// set arguments
		for i := range nargsProvided {
			v := rt.Stack[len(rt.Stack)-1]
			rt.Stack = rt.Stack[:len(rt.Stack)-1]

			index := nargsProvided - i - 1
			*rt.Locals[base+index] = v
		}

		// save old state
		rt.PushFrame()

		// prepare new frame
		rt.CallFrame.Base = base
		rt.CallFrame.Captured = fn.Captured
		rt.CallFrame.Locals = fn.Capacity

		// jump to function code
		offset := fn.Start - rt.CallFrame.Ip
		return offset, nil
	}

	return 1, core.CustomError("cannot call a non-function '%v'", value)
}
