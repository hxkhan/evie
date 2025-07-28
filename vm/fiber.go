package vm

/* IDEA:

type fiber struct {
	active *UserFn  // currently active user function
	stack  []Value  // a flat shared stack for local variables in the current call stack
	base   int      // where locals of the active function start at
}

functions only allocate non-escaping locals on the stack
the rest go on the heap as free variables somehow

*/

type fiber struct {
	active *UserFn  // currently active user function
	stack  []*Value // flat shared stack for local variables in the current call stack
	base   int      // where locals of the active function start at
}

func (fbr *fiber) getLocal(index local) Value {
	return *(fbr.stack[fbr.base+int(index)])
}

func (fbr *fiber) storeLocal(index local, value Value) {
	*(fbr.stack[fbr.base+int(index)]) = value
}

func (fbr *fiber) getCaptured(index captured) Value {
	return *(fbr.active.references[index])
}

func (fbr *fiber) storeCaptured(index captured, value Value) {
	*(fbr.active.references[index]) = value
}

func (fbr *fiber) getLocalByRef(index int) *Value {
	return fbr.stack[fbr.base+index]
}

func (fbr *fiber) getCapturedByRef(index int) *Value {
	return fbr.active.references[index]
}

func (fbr *fiber) popStack(n int) {
	fbr.stack = fbr.stack[:len(fbr.stack)-n]
}

func (fbr *fiber) swapBase(base int) (old int) {
	old = fbr.base
	fbr.base = base
	return old
}

func (fbr *fiber) swapActive(new *UserFn) (old *UserFn) {
	old = fbr.active
	fbr.active = new
	return old
}

func (fbr *fiber) tryNativeCall(value Value, args []instruction) (result Value, exc *Exception) {
	nfn, ok := value.AsGoFunc()
	if !ok {
		return Value{}, notFunction
	}

	switch len(args) {
	case 0:
		if fn, ok := nfn.(func() (Value, *Exception)); ok {
			return fn()
		}
	case 1:
		if fn, ok := nfn.(func(Value) (Value, *Exception)); ok {
			arg0, err := args[0](fbr)
			if err != nil {
				return arg0, err
			}
			return fn(arg0)
		}
	case 2:
		if fn, ok := nfn.(func(Value, Value) (Value, *Exception)); ok {
			arg0, err := args[0](fbr)
			if err != nil {
				return arg0, err
			}
			arg1, err := args[1](fbr)
			if err != nil {
				return arg1, err
			}

			return fn(arg0, arg1)
		}
	case 3:
		if fn, ok := nfn.(func(Value, Value, Value) (Value, *Exception)); ok {
			arg0, err := args[0](fbr)
			if err != nil {
				return arg0, err
			}
			arg1, err := args[1](fbr)
			if err != nil {
				return arg1, err
			}
			arg2, err := args[2](fbr)
			if err != nil {
				return arg2, err
			}

			return fn(arg0, arg1, arg2)
		}
	}

	panic("this cant be")
}
