package vm

/* IDEA: Fix the missing-variable-in-inner-closure issue

type fiber struct {
	active *UserFn  // currently active user function
	stack  []Value  // a flat shared stack for local variables in the current call stack
	base   int      // where locals of the active function start at
}

functions only allocate non-escaping locals on the stack
the rest go on the heap as free variables somehow

*/

/* IDEA2: Fix the missing-variable-in-inner-closure issue by moving to propagate captured variables down

fn test() {
    y := 20
    return fn() {         // ← getter
        return fn() {     // ← printer
            echo y
        }
    }
}

--- Gets turned into

fn test() {
    y := 20
    return fn() {         // ← getter
		y := y
        return fn() {     // ← printer
            echo y
        }
    }
}

*/

type fiber struct {
	active *UserFn  // currently active user function
	stack  []*Value // a flat shared stack for local variables in the current call stack
	base   int      // where locals of the active function start at
}

func (fbr *fiber) getLocal(index int) Value {
	return *(fbr.stack[fbr.base+index])
}

func (fbr *fiber) storeLocal(index int, value Value) {
	*(fbr.stack[fbr.base+index]) = value
}

func (fbr *fiber) getCaptured(index int) Value {
	return *(fbr.active.captured[index])
}

func (fbr *fiber) storeCaptured(index int, value Value) {
	*(fbr.active.captured[index]) = value
}

// what happens when we capture a captured??
func (fbr *fiber) capture(ref reference) *Value {
	outerFn := fbr.active
	for range ref.scroll - 1 {
		outerFn = outerFn.outer
	}
	return fbr.stack[outerFn.baseSnapshot+ref.index]
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
