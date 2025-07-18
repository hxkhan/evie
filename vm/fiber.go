package vm

type fiber struct {
	active *UserFn  // currently active user function
	stack  []*Value // stack for local variables accessible in the current call stack
	basis  []int    // one base per function; basis[-1] is where the current function's locals start at
}

func (fbr *fiber) storeLocal(index int, value Value) {
	*(fbr.stack[fbr.getCurrentBase()+index]) = value
}

func (fbr *fiber) storeCaptured(index int, value Value) {
	*(fbr.active.captured[index]) = value
}

func (fbr *fiber) getLocal(index int) Value {
	return *(fbr.stack[fbr.getCurrentBase()+index])
}

func (fbr *fiber) capture(index int, scroll int) *Value {
	return fbr.stack[fbr.getScrolledBase(scroll)+index]
}

func (fbr *fiber) getCaptured(index int) Value {
	return *(fbr.active.captured[index])
}

func (fbr *fiber) getCurrentBase() int {
	return fbr.basis[len(fbr.basis)-1]
}

func (fbr *fiber) getScrolledBase(scroll int) int {
	return fbr.basis[len(fbr.basis)-scroll]
}

func (fbr *fiber) pushBase(base int) {
	fbr.basis = append(fbr.basis, base)
}

func (fbr *fiber) popBase() {
	fbr.basis = fbr.basis[:len(fbr.basis)-1]
}

func (fbr *fiber) pushLocal(v *Value) {
	fbr.stack = append(fbr.stack, v)
}

func (fbr *fiber) popLocals(n int) {
	fbr.stack = fbr.stack[:len(fbr.stack)-n]
}

func (fbr *fiber) stackSize() int {
	return len(fbr.stack)
}

func (fbr *fiber) swapActive(new *UserFn) (old *UserFn) {
	old = fbr.active
	fbr.active = new
	return old
}
