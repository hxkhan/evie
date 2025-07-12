package core

import (
	"sync"
)

func NewMachine(builtins []Value) Machine {
	return Machine{
		Boxes:      make(pool[Value], 0, 48),    // starting with a capacity of storing 48 boxed values
		Coroutines: make(pool[CoRoutine], 0, 3), // starting with a capacity of storing 3 co-routines
		Builtins:   builtins,                    // built-in variables
	}
}

type Machine struct {
	Code []Instruction

	Boxes      pool[Value]     // pool of boxes for values
	Coroutines pool[CoRoutine] // pool of co-routines

	Globals  []*Value // globally scoped variables declared by the user
	Builtins []Value  // built-in scope; can get from but can't set in

	trace []string // call stack trace

	gil sync.Mutex     // global interpreter lock
	wg  sync.WaitGroup // wait for all threads
}

type CoRoutine struct {
	CallFrame             // current frame
	Locals    []*Value    // local variables accessible in the current call stack
	Stack     []Value     // data-stack for the current coroutine
	CallStack []CallFrame // one base per function
}

type CallFrame struct {
	Ip       int      // instruction pointer
	Captured []*Value // captured variables of the current function being executed
	Base     int      // base index in the Locals for the current function
	Locals   int      // how many locals it owns in CoRoutine.Locals
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
	*(rt.Locals[rt.Base+index]) = value
}

func (rt *CoRoutine) StoreCaptured(index int, value Value) {
	*(rt.Captured[index]) = value
}

func (rt *CoRoutine) GetLocal(index int) Value {
	return *(rt.Locals[rt.Base+index])
}

func (rt *CoRoutine) Capture(index int, scroll int) *Value {
	return rt.Locals[rt.GetScrolledBase(scroll)+index]
}

func (rt *CoRoutine) GetCaptured(index int) Value {
	return *(rt.Captured[index])
}

func (rt *CoRoutine) GetScrolledBase(scroll int) int {
	if scroll == 1 {
		return rt.CallFrame.Base
	}
	return rt.CallStack[len(rt.CallStack)-scroll+1].Base
}

func (rt *CoRoutine) PushFrame() {
	rt.CallStack = append(rt.CallStack, rt.CallFrame)
}

func (rt *CoRoutine) PopFrame(vm *Machine) {
	// cleanup current
	for index := range rt.CallFrame.Locals {
		// free all for now
		vm.Boxes.Put(rt.Locals[rt.CallFrame.Base+index])
	}
	rt.PopLocals(rt.CallFrame.Locals)

	// restore top
	rt.CallFrame = rt.CallStack[len(rt.CallStack)-1]
	rt.CallStack = rt.CallStack[:len(rt.CallStack)-1]
}

func (rt *CoRoutine) PopLocals(n int) {
	rt.Locals = rt.Locals[:len(rt.Locals)-n]
}

type Reference struct {
	Index  int
	Scroll int
}

type FuncInfoStatic struct {
	Name        string      // name of the function
	Args        []string    // argument names
	Refs        []Reference // captured references
	NonEscaping []int       // the locals that do not escape
	Capacity    int         // total required scope-capacity
	Start       int         // the start of the function code
	Size        int         // the size of the function code
	Machine     *Machine    // the corresponding vm
}

type UserFn struct {
	*FuncInfoStatic
	Captured []*Value
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
