package core

import (
	"sync"

	"github.com/hk-32/evie/pool"
)

func NewMachine(builtins []Value) Machine {
	return Machine{
		Boxes:    pool.Make[Value](48), // starting with a capacity of storing 48 boxed values
		Fibers:   pool.Make[Fiber](3),  // starting with a capacity of storing 3 co-routines
		Builtins: builtins,             // built-in variables
	}
}

type Machine struct {
	Boxes  pool.Instance[Value] // pooled boxes for this vm
	Fibers pool.Instance[Fiber] // pooled fibers for this vm

	Globals  []*Value // globally scoped variables declared by the user
	Builtins []Value  // built-in scope; can get from but can't set in

	trace []string // call-stack trace

	gil sync.Mutex     // global interpreter lock
	wg  sync.WaitGroup // wait for all fibers to complete
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

type Fiber struct {
	Active *UserFn  // currently active user function
	Memory []*Value // stack for local variables accessible in the current call stack
	Stack  []*Value // stack for local variables accessible in the current call stack
	Basis  []int    // one base per function; basis[-1] is where the current function's locals start at
}

/* func (rt *CoRoutine) newRoutine(ip int, locals []*Value, captured []*Value) *CoRoutine {
	return &CoRoutine{ip, fbr.vm, fbr.code, captured, locals, []int{0}}
} */

/* func (rt *Routine) terminate() {
	m.wg.Done()
	ReleaseGIL()
} */

func (fbr *Fiber) StoreLocal(index int, value Value) {
	*(fbr.Stack[fbr.GetCurrentBase()+index]) = value
}

func (fbr *Fiber) StoreCaptured(index int, value Value) {
	*(fbr.Active.Captured[index]) = value
}

func (fbr *Fiber) GetLocal(index int) Value {
	return *(fbr.Stack[fbr.GetCurrentBase()+index])
}

func (fbr *Fiber) Capture(index int, scroll int) *Value {
	return fbr.Stack[fbr.GetScrolledBase(scroll)+index]
}

func (fbr *Fiber) GetCaptured(index int) Value {
	return *(fbr.Active.Captured[index])
}

func (fbr *Fiber) GetCurrentBase() int {
	return fbr.Basis[len(fbr.Basis)-1]
}

func (fbr *Fiber) GetScrolledBase(scroll int) int {
	return fbr.Basis[len(fbr.Basis)-scroll]
}

func (fbr *Fiber) PushBase(base int) {
	fbr.Basis = append(fbr.Basis, base)
}

func (fbr *Fiber) PopBase() {
	fbr.Basis = fbr.Basis[:len(fbr.Basis)-1]
}

func (fbr *Fiber) PushLocal(v *Value) {
	fbr.Stack = append(fbr.Stack, v)
}

func (fbr *Fiber) PopLocals(n int) {
	fbr.Stack = fbr.Stack[:len(fbr.Stack)-n]
}

func (fbr *Fiber) StackSize() int {
	return len(fbr.Stack)
}

func (fbr *Fiber) SwapActive(new *UserFn) (old *UserFn) {
	old = fbr.Active
	fbr.Active = new
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

type Instruction func(fbr *Fiber) (Value, error)

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
	fbr := vm.Fibers.Get()
	fbr.Active = fn
	fbr.Basis = fbr.Basis[:0]
	fbr.Stack = fbr.Stack[:0]

	// create space for all the locals
	for range fn.Capacity {
		fbr.PushLocal(vm.Boxes.Get())
	}

	// set arguments
	for idx, arg := range args {
		*fbr.Stack[idx] = arg
	}

	// prep for execution & save currently captured values
	fbr.PushBase(0)
	result, err = fn.Code(fbr)

	// release non-escaping locals
	for _, idx := range fn.NonEscaping {
		vm.Boxes.Put(fbr.Stack[idx])
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
