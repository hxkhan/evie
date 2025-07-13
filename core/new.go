package core

type Instruction func(rt *CoRoutine) (Value, error)

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
	rt.Basis = []int{0}
	rt.Captured = fn.Captured

	// allocate space on stack for arguments & local variables
	rt.Stack = make([]*Value, fn.Capacity)
	for i := range fn.Capacity {
		rt.Stack[i] = vm.Boxes.New()
	}

	// set arguments
	for i, v := range args {
		*rt.Stack[i] = v
	}

	// run function code
	value, err := fn.Code(rt)

	// release non-escaping locals
	for _, index := range fn.NonEscaping {
		vm.Boxes.Put(rt.Stack[index])
	}

	// don't implicitly return the return value of the last executed instruction
	switch err {
	case nil:
		return Value{}, nil
	case ErrReturnSignal:
		return value, nil
	default:
		return value, ErrWithTrace{err, vm.trace}
	}
}

func (rt *CoRoutine) ExitUserFN(popLocals int, oldEnc []*Value) {
	// return to caller context
	rt.PopLocals(popLocals)
	rt.PopBase()
	rt.Captured = oldEnc
}
