package vm

import (
	"sync/atomic"
	"unsafe"
)

// fiber is one thread of execution inside an Instance. At most one fiber of an
// instance runs evie code at a time; the one that does is the instance's running fiber.
type fiber struct {
	vm      *Instance      // the instance that owns this fiber
	running bool           // this fiber currently holds the instance runner
	active  *userFn        // currently active user function
	stack   []Value        // flat shared stack for local variables in the current call stack
	base    int            // where locals of the active function start at
	stop    *stopper       // says when this fiber has to stop
	signal  *atomic.Uint32 // the instance's stop signal, held here to save a load on every check
	seen    uint32         // the signal when this fiber last looked
}

const initialStackCap = 256

func newFiber(vm *Instance) *fiber {
	return &fiber{vm: vm, signal: &vm.rt.signal, stack: make([]Value, 0, initialStackCap)}
}

// lock makes this fiber the instance's running fiber. It reports whether the
// runner was acquired here; false means the fiber was already running.
func (fbr *fiber) lock() bool {
	if fbr.running {
		return false
	}
	fbr.vm.rt.acquire()
	fbr.running = true
	return true
}

// unlock gives the instance runner back
func (fbr *fiber) unlock() {
	fbr.running = false
	fbr.vm.rt.release()
}

// reset forgets everything from the previous use of this fiber
func (fbr *fiber) reset() {
	clear(fbr.stack)
	fbr.running = false
	fbr.active = nil
	fbr.base = 0
	fbr.stack = fbr.stack[:0]
}

// runGuarded executes one instruction on this fiber, turning a panic into an exception
func (fbr *fiber) runGuarded(code instruction) (result Value, exc exception) {
	stack, base, active := len(fbr.stack), fbr.base, fbr.active
	defer func() {
		if r := recover(); r != nil {
			result, exc = Value{}, recovered(r)
			fbr.unwind(stack, base, active)
		}
	}()
	return code(fbr)
}

// unwind puts the fiber back into the state it had when a host call began,
// after a panic skipped the frame bookkeeping.
func (fbr *fiber) unwind(stack int, base int, active *userFn) {
	clear(fbr.stack[stack:])
	fbr.stack = fbr.stack[:stack]
	fbr.base = base
	fbr.active = active
}

func (fbr *fiber) allocFrame(n int) int {
	base := len(fbr.stack)
	end := base + n
	if end <= cap(fbr.stack) {
		fbr.stack = fbr.stack[:end]
	} else {
		fbr.growStack(end)
	}
	return base
}

func (fbr *fiber) growStack(end int) {
	newCap := max(cap(fbr.stack)*2, end, initialStackCap)
	grown := make([]Value, end, newCap)
	copy(grown, fbr.stack)
	fbr.stack = grown
}

func (fbr *fiber) boxParams(base int, slots []int) {
	for _, slot := range slots {
		cell := new(Value)
		*cell = fbr.stack[base+slot]
		fbr.stack[base+slot] = Value{pointer: unsafe.Pointer(cell)}
	}
}

func (fbr *fiber) get(binding local) *Value {
	if !binding.isCaptured {
		if !binding.isBoxed {
			return &fbr.stack[fbr.base+int(binding.index)]
		}
		return (*Value)(fbr.stack[fbr.base+int(binding.index)].pointer)
	}
	return fbr.active.references[binding.index]
}

func (fbr *fiber) set(binding local, value Value) {
	if !binding.isCaptured {
		if !binding.isBoxed {
			fbr.stack[fbr.base+int(binding.index)] = value
			return
		}
		*(*Value)(fbr.stack[fbr.base+int(binding.index)].pointer) = value
		return
	}
	*(fbr.active.references[int(binding.index)]) = value
}

func (fbr *fiber) getLocal(index int16) Value {
	return fbr.stack[fbr.base+int(index)]
}

func (fbr *fiber) getBoxedLocal(index int16) Value {
	return *(*Value)(fbr.stack[fbr.base+int(index)].pointer)
}

func (fbr *fiber) setLocal(index int, value Value) {
	fbr.stack[fbr.base+index] = value
}

func (fbr *fiber) setEscapingLocal(index int, value Value) {
	cell := new(Value)
	*cell = value
	fbr.stack[fbr.base+index] = Value{pointer: unsafe.Pointer(cell)}
}

func (fbr *fiber) newEscapingLocal(index int) *Value {
	cell := new(Value)
	fbr.stack[fbr.base+index] = Value{pointer: unsafe.Pointer(cell)}
	return cell
}

func (fbr *fiber) getCaptured(index int16) Value {
	return *(fbr.active.references[index])
}

func (fbr *fiber) getLocalByRef(index int) *Value {
	return (*Value)(fbr.stack[fbr.base+index].pointer)
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

func (fbr *fiber) swapActive(new *userFn) (old *userFn) {
	old = fbr.active
	fbr.active = new
	return old
}
