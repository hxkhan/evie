package vm

type Fiber struct {
	vm     *Instance // the instance that spawned this fiber
	synced bool      // run in synchronized mode or not
	active *UserFn   // currently active user function
	stack  []*Value  // flat shared stack for local variables in the current call stack
	base   int       // where locals of the active function start at
	boxes  []Value   // pooled boxes for this fiber
}

func (fbr *Fiber) get(binding local) *Value {
	if !binding.isCaptured {
		return fbr.stack[fbr.base+int(binding.index)]
	}
	return fbr.active.references[binding.index]
}

func (fbr *Fiber) set(binding local, value Value) {
	if !binding.isCaptured {
		*(fbr.stack[fbr.base+int(binding.index)]) = value
		return
	}
	*(fbr.active.references[int(binding.index)]) = value
}

func (fbr *Fiber) GetLocal(index int16) Value {
	return *(fbr.stack[fbr.base+int(index)])
}

func (fbr *Fiber) setLocal(index int, value Value) {
	*(fbr.stack[fbr.base+index]) = value
}

func (fbr *Fiber) getCaptured(index int16) Value {
	return *(fbr.active.references[index])
}

func (fbr *Fiber) getLocalByRef(index int) *Value {
	return fbr.stack[fbr.base+index]
}

func (fbr *Fiber) getCapturedByRef(index int) *Value {
	return fbr.active.references[index]
}

func (fbr *Fiber) popStack(n int) {
	fbr.stack = fbr.stack[:len(fbr.stack)-n]
}

func (fbr *Fiber) swapBase(base int) (old int) {
	old = fbr.base
	fbr.base = base
	return old
}

func (fbr *Fiber) swapActive(new *UserFn) (old *UserFn) {
	old = fbr.active
	fbr.active = new
	return old
}

func (fbr *Fiber) pop() (v *Value) {
	if len(fbr.boxes) == 0 {
		return new(Value)
	}

	top := len(fbr.boxes) - 1
	v = &fbr.boxes[top]
	fbr.boxes = fbr.boxes[:top]
	return v
}

func (fbr *Fiber) push(amount int) {
	size := len(fbr.boxes) + amount
	if size <= cap(fbr.boxes) {
		fbr.boxes = fbr.boxes[:size]
	}
}
