package core

type Instruction func(rt *CoRoutine) (int, error)

func (fn *UserFn) Call(args ...Value) (Value, error) {
	if len(fn.Args) != len(args) {
		if fn.Name != "λ" {
			return Value{}, CustomError("function '%v' requires %v argument(s), %v provided", fn.Name, len(fn.Args), len(args))
		}
		return Value{}, CustomError("function requires %v argument(s), %v provided", len(fn.Args), len(args))
	}

	vm := fn.Machine
	vm.AcquireGIL()
	defer vm.ReleaseGIL()

	// fetch a coroutine and prepare it
	rt := vm.Coroutines.New()
	rt.CallFrame.Base = 0
	rt.CallFrame.Captured = fn.Captured
	rt.CallFrame.Locals = fn.Capacity

	// allocate space on stack for arguments & local variables
	rt.Locals = make([]*Value, fn.Capacity)
	for i := range fn.Capacity {
		rt.Locals[i] = vm.Boxes.New()
	}

	// set arguments
	for i, v := range args {
		*rt.Locals[i] = v
	}

	var errors error
	rt.CallStack = append(rt.CallStack, CallFrame{Ip: len(vm.Code)})

	// run function code
	rt.Ip = fn.Start
	for rt.Ip < len(vm.Code) {
		i, err := vm.Code[rt.Ip](rt)
		if err != nil {
			errors = err
			break
		}

		rt.Ip += i
	}

	// free coroutine
	vm.Coroutines.Put(rt)

	// don't implicitly return the return value of the last executed instruction
	switch errors {
	case nil:
		return Value{}, nil
	default:
		return Value{}, errors
	}
}
