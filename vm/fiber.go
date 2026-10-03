package vm

// Fiber is one thread of execution inside an Instance. At most one fiber of an
// instance runs evie code at a time; the one that does is the instance's running fiber.
type Fiber struct {
	vm      *Instance // the instance that owns this fiber
	running bool      // this fiber currently holds the instance runner
	active  *UserFn   // currently active user function
	stack   []*Value  // flat shared stack for local variables in the current call stack
	base    int       // where locals of the active function start at
	boxes   []Value   // current chunk of pooled boxes: boxes[:len] are free, boxes[len:cap] are in use
	spent   [][]Value // chunks whose boxes are all in use, oldest first
	spare   [][]Value // chunks whose boxes are all free, kept for reuse
}

// boxChunk is the number of boxes allocated at once for a fiber's pool
const boxChunk = 256

func newFiber(vm *Instance) *Fiber {
	return &Fiber{vm: vm, boxes: make([]Value, boxChunk)}
}

// lock makes this fiber the instance's running fiber. It reports whether the
// runner was acquired here; false means the fiber was already running.
func (fbr *Fiber) lock() bool {
	if fbr.running {
		return false
	}
	fbr.vm.rt.acquire()
	fbr.running = true
	return true
}

// unlock gives the instance runner back
func (fbr *Fiber) unlock() {
	fbr.running = false
	fbr.vm.rt.release()
}

// reset forgets everything from the previous use of this fiber
func (fbr *Fiber) reset() {
	fbr.running = false
	fbr.active = nil
	fbr.base = 0
	fbr.stack = fbr.stack[:0]
}

// runGuarded executes one instruction on this fiber, turning a panic into an exception
func (fbr *Fiber) runGuarded(code instruction) (result Value, exc Exception) {
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
// after a panic skipped the frame bookkeeping. The box pool cannot be
// reconciled, so it starts over; the boxes still referenced stay alive.
func (fbr *Fiber) unwind(stack int, base int, active *UserFn) {
	fbr.stack = fbr.stack[:stack]
	fbr.base = base
	fbr.active = active
	fbr.boxes = make([]Value, boxChunk)
	fbr.spent = nil
	fbr.spare = nil
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

func (fbr *Fiber) getLocal(index int16) Value {
	if fbr.base+int(index) < len(fbr.stack) {
		return *(fbr.stack[fbr.base+int(index)])
	}
	return Value{}
}

func (fbr *Fiber) setLocal(index int, value Value) {
	for len(fbr.stack) <= fbr.base+index {
		fbr.stack = append(fbr.stack, &Value{})
	}
	*(fbr.stack[fbr.base+index]) = value
}

func (fbr *Fiber) getCaptured(index int16) Value {
	return *(fbr.active.references[index])
}

func (fbr *Fiber) getLocalByRef(index int) *Value {
	for len(fbr.stack) <= fbr.base+index {
		fbr.stack = append(fbr.stack, &Value{})
	}
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

// pop hands out one box from the pool, opening a new chunk when the current one is used up
func (fbr *Fiber) pop() (v *Value) {
	if len(fbr.boxes) == 0 {
		fbr.spent = append(fbr.spent, fbr.boxes)
		if n := len(fbr.spare); n > 0 {
			fbr.boxes = fbr.spare[n-1]
			fbr.spare = fbr.spare[:n-1]
		} else {
			fbr.boxes = make([]Value, boxChunk)
		}
	}

	top := len(fbr.boxes) - 1
	v = &fbr.boxes[top]
	fbr.boxes = fbr.boxes[:top]
	return v
}

// push gives the most recently handed out boxes back to the pool
func (fbr *Fiber) push(amount int) {
	for {
		room := cap(fbr.boxes) - len(fbr.boxes)
		if amount <= room {
			fbr.boxes = fbr.boxes[:len(fbr.boxes)+amount]
			return
		}
		amount -= room

		// this chunk is entirely free again; keep it for later and continue in the previous one
		fbr.spare = append(fbr.spare, fbr.boxes[:cap(fbr.boxes)])
		n := len(fbr.spent)
		fbr.boxes = fbr.spent[n-1]
		fbr.spent = fbr.spent[:n-1]
	}
}
