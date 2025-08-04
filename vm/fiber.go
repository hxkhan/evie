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

func (fbr *fiber) get(v local) Value {
	if v.isCaptured {
		return *(fbr.active.references[v.index])
	}
	return *(fbr.stack[fbr.base+v.index])
}

func (fbr *fiber) getLocal(index int) Value {
	return *(fbr.stack[fbr.base+index])
}

func (fbr *fiber) storeLocal(index int, value Value) {
	*(fbr.stack[fbr.base+index]) = value
}

func (fbr *fiber) getCaptured(index int) Value {
	return *(fbr.active.references[index])
}

func (fbr *fiber) storeCaptured(index int, value Value) {
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

func (fbr *fiber) tryNonStandardCall(value Value, arguments []instruction) (result Value, exc *Exception) {
	// check if it is a go function
	if fn, isGoFunc := value.AsGoFunc(); isGoFunc {
		if fn.nargs != len(arguments) {
			return Value{}, CustomError("function requires %v argument(s), %v provided", fn.nargs, len(arguments))
		}

		switch fn.nargs {
		case -1:
			panic("variadic functions not supported yet")
		case 0:
			function := *(*func() (Value, *Exception))(fn.ptr)
			return function()
		case 1:
			function := *(*func(Value) (Value, *Exception))(fn.ptr)
			arg0, err := arguments[0](fbr)
			if err != nil {
				return arg0, err
			}
			return function(arg0)
		case 2:
			function := *(*func(Value, Value) (Value, *Exception))(fn.ptr)
			arg0, err := arguments[0](fbr)
			if err != nil {
				return arg0, err
			}

			arg1, err := arguments[1](fbr)
			if err != nil {
				return arg0, err
			}
			return function(arg0, arg1)
		}

		panic("unsuported call")
	}

	// check if it is a method and owner combo
	if m, ok := value.asMethod(); ok {
		fn, ok := m.fn.AsGoFunc()
		if !ok {
			return Value{}, notFunction
		}

		switch fn.nargs {
		case -1:
			panic("variadic functions not supported yet")
		case 0:
			panic("how did we get a method that does not even take itself as an arguement?")
		case 1:
			function := *(*func(Value) (Value, *Exception))(fn.ptr)
			return function(m.this)
		case 2:
			function := *(*func(Value, Value) (Value, *Exception))(fn.ptr)
			arg0, err := arguments[0](fbr)
			if err != nil {
				return arg0, err
			}
			return function(m.this, arg0)
		}

		panic("unsuported call")
	}

	return Value{}, CustomError("cannot call a non-function '%v'", value)
}

func (fbr *fiber) call(fn *goFunc, arguments []instruction) (result Value, exc *Exception) {
	if fn.nargs != len(arguments) {
		return Value{}, CustomError("function requires %v argument(s), %v provided", fn.nargs, len(arguments))
	}

	switch fn.nargs {
	case -1:
		panic("variadic functions not supported yet")
	case 0:
		function := *(*func() (Value, *Exception))(fn.ptr)
		return function()
	case 1:
		function := *(*func(Value) (Value, *Exception))(fn.ptr)
		arg0, err := arguments[0](fbr)
		if err != nil {
			return arg0, err
		}
		return function(arg0)
	case 2:
		function := *(*func(Value, Value) (Value, *Exception))(fn.ptr)
		arg0, err := arguments[0](fbr)
		if err != nil {
			return arg0, err
		}

		arg1, err := arguments[1](fbr)
		if err != nil {
			return arg0, err
		}
		return function(arg0, arg1)
	}

	panic("unsuported call")
}
