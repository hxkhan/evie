package core

import "sync"

func NewMachine(builtins []Value) Machine {
	return Machine{
		Boxes:      make(pool[Value], 0, 48),    // starting with a capacity of storing 48 boxed values
		Coroutines: make(pool[CoRoutine], 0, 3), // starting with a capacity of storing 3 co-routines
		Builtins:   builtins,                    // built-in variables
	}
}

type Machine struct {
	Boxes      pool[Value]     // pool of boxes for values
	Coroutines pool[CoRoutine] // pool of co-routines

	Globals  []*Value // globally scoped variables declared by the user
	Builtins []Value  // built-in scope; can get from but can't set in

	trace []string // call-stack trace

	gil sync.Mutex     // global interpreter lock
	wg  sync.WaitGroup // wait for all threads
}

type CoRoutine struct {
	Captured []*Value // captured variables of the current function being executed
	Stack    []*Value // stack for local variables accessible in the current call stack
	Basis    []int    // one base per function; basis[-1] is where the current function's locals start at
}

func (m *Machine) WaitForNoActivity() {
	m.wg.Wait()
}

func (m *Machine) ReleaseGIL() {
	m.gil.Unlock()
}

func (m *Machine) AcquireGIL() {
	m.gil.Lock()
}

/* func (rt *CoRoutine) newRoutine(ip int, locals []*Value, captured []*Value) *CoRoutine {
	return &CoRoutine{ip, rt.vm, rt.code, captured, locals, []int{0}}
} */

/* func (rt *Routine) terminate() {
	m.wg.Done()
	ReleaseGIL()
} */

func (rt *CoRoutine) StoreLocal(index int, value Value) {
	*(rt.Stack[rt.GetCurrentBase()+index]) = value
}

func (rt *CoRoutine) StoreCaptured(index int, value Value) {
	*(rt.Captured[index]) = value
}

func (rt *CoRoutine) GetLocal(index int) Value {
	return *(rt.Stack[rt.GetCurrentBase()+index])
}

func (rt *CoRoutine) Capture(index int, scroll int) *Value {
	return rt.Stack[rt.GetScrolledBase(scroll)+index]
}

func (rt *CoRoutine) GetCaptured(index int) Value {
	return *(rt.Captured[index])
}

func (rt *CoRoutine) GetCurrentBase() int {
	return rt.Basis[len(rt.Basis)-1]
}

func (rt *CoRoutine) GetScrolledBase(scroll int) int {
	return rt.Basis[len(rt.Basis)-scroll]
}

func (rt *CoRoutine) PushBase(base int) {
	rt.Basis = append(rt.Basis, base)
}

func (rt *CoRoutine) PopBase() {
	rt.Basis = rt.Basis[:len(rt.Basis)-1]
}

func (rt *CoRoutine) PushLocal(v *Value) {
	rt.Stack = append(rt.Stack, v)
}

func (rt *CoRoutine) PopLocals(n int) {
	rt.Stack = rt.Stack[:len(rt.Stack)-n]
}

func (rt *CoRoutine) StackSize() int {
	return len(rt.Stack)
}

func (rt *CoRoutine) SwapCaptured(new []*Value) (old []*Value) {
	old = rt.Captured
	rt.Captured = new
	return old
}

type Reference struct {
	Index  int
	Scroll int
}

func (ref Reference) IsLocal() bool {
	return ref.Scroll == 0
}

func (ref Reference) IsCaptured() bool {
	return ref.Scroll > 0
}

type Instruction func(rt *CoRoutine) (Value, error)

type FuncInfoStatic struct {
	Name        string      // name of the function
	Args        []string    // argument names
	Refs        []Reference // captured references
	NonEscaping []int       // the locals that do not escape
	Capacity    int         // total required scope-capacity
	Code        Instruction // the actual function code
	Machine     *Machine    // the corresponding vm
}

type UserFn struct {
	*FuncInfoStatic
	Captured []*Value
}

func (fn *UserFn) Call(args ...Value) (result Value, err error) {
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
	rt.Captured = fn.Captured
	rt.Basis = rt.Basis[:0]
	rt.Stack = rt.Stack[:0]

	// create space for all the locals
	for range fn.Capacity {
		rt.PushLocal(vm.Boxes.New())
	}

	// set arguments
	for idx, arg := range args {
		*rt.Stack[idx] = arg
	}

	// prep for execution & save currently captured values
	rt.PushBase(0)
	result, err = fn.Code(rt)

	// release non-escaping locals
	for _, idx := range fn.NonEscaping {
		vm.Boxes.Put(rt.Stack[idx])
	}

	// don't implicitly return the return value of the last executed instruction
	switch err {
	case nil:
		return Value{}, nil
	case ErrReturnSignal:
		return result, nil
	default:
		return result, err
	}
}

func (fn UserFn) String() string {
	return "<function>"
}

func NewTask(fn func() (Value, error)) Value {
	task := make(chan TaskResult, 1)
	go func() {
		res, err := fn()
		task <- TaskResult{res, err}
		close(task)
	}()
	return BoxTask(task)
}

type TaskResult struct {
	Result Value
	Error  error
}

type Tuple []any
