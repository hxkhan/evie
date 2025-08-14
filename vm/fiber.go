package vm

import "github.com/hxkhan/evie/ds"

type fiber struct {
	unsynced bool             // run in synchronized mode or not
	active   *UserFn          // currently active user function
	stack    []*Value         // flat shared stack for local variables in the current call stack
	base     int              // where locals of the active function start at
	boxes    ds.Slice[*Value] // pooled boxes for this fiber
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

func (fbr *fiber) newValue() (obj *Value) {
	if fbr.boxes.IsEmpty() {
		return new(Value)
	}
	return fbr.boxes.Pop()
}

func (fbr *fiber) putValue(obj *Value) {
	if fbr.boxes.Len() < fbr.boxes.Cap() {
		fbr.boxes.Push(obj)
	}
}
