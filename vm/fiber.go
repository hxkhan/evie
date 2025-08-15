package vm

type fiber struct {
	vm             *Instance // the instance that spawned this fiber
	unsynchronized bool      // run in unsynchronized mode or not
	active         *UserFn   // currently active user function
	stack          []*Value  // flat shared stack for local variables in the current call stack
	base           int       // where locals of the active function start at
	boxes          []Value   // pooled boxes for this fiber
}

func (fbr *fiber) synced() bool {
	return !fbr.unsynchronized
}

func (fbr *fiber) unsynced() bool {
	return fbr.unsynchronized
}

func (fbr *fiber) get(v local) Value {
	if v.isCaptured {
		return *(fbr.active.references[v.index])
	}
	return *(fbr.stack[fbr.base+v.index])
}

func (fbr *fiber) getByRef(v local) *Value {
	if v.isCaptured {
		return fbr.active.references[v.index]
	}
	return fbr.stack[fbr.base+v.index]
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

func (fbr *fiber) pop() (v *Value) {
	if len(fbr.boxes) == 0 {
		return new(Value)
	}

	top := len(fbr.boxes) - 1
	v = &fbr.boxes[top]
	fbr.boxes = fbr.boxes[:top]
	return v
}

func (fbr *fiber) push(amount int) {
	size := len(fbr.boxes) + amount
	if size <= cap(fbr.boxes) {
		fbr.boxes = fbr.boxes[:size]
	}
}
