package core

import "sync"

type InfoSource interface {
	GetSymbolName(ip int) (symbol string, exists bool) // Only used when errors occur
	GetFuncInfo(ip int) (info *FuncInfo, exists bool)  // Used to get vital function information upon creation
}

func NewMachine(builtins []Value, infoSource InfoSource) *Machine {
	return &Machine{
		boxes:      make(pool[Value], 0, 48),    // starting with a capacity of storing 48 boxed values
		coroutines: make(pool[CoRoutine], 0, 3), // starting with a capacity of storing 3 co-routines
		builtins:   builtins,
		infoSource: infoSource,
	}
}

func (m *Machine) Run(code []byte, numGlobals int) (Value, error) {
	// starting point for the code execution
	start := max(len(m.code), 0)
	m.code = code

	m.AcquireGIL()
	defer m.ReleaseGIL()

	// ensure that the globals are large enough
	if len(m.globals) < numGlobals {
		for range numGlobals - len(m.globals) {
			m.globals = append(m.globals, m.boxes.new())
		}
	}

	// fetch a coroutine and prepare it
	rt := m.coroutines.new()
	rt.vm = m
	rt.code = m.code
	rt.stack = m.globals
	rt.basis = []int{0}

	// start code execution
	for rt.ip = start; rt.ip < len(m.code); rt.ip++ {
		// fetch and execute the instruction
		if _, err := instructions[m.code[rt.ip]](rt); err != nil {
			return Value{}, err
		}
	}

	return Value{}, nil
}

func (m *Machine) GetGlobal(address int) *Value {
	return m.globals[address]
}

type Reference struct {
	Index  int
	Scroll int
}

type FuncInfo struct {
	Name        string      // name of the function
	Args        []string    // argument names
	Refs        []Reference // captured references
	NonEscaping []int       // the locals that do not escape
	Capacity    int         // total required scope-capacity
	Start       int         // entry point index
	End         int         // associated op.END index
}

type Machine struct {
	code []byte // executable bytes

	boxes      pool[Value]     // pool of boxes for values
	coroutines pool[CoRoutine] // pool of co-routines

	globals  []*Value
	builtins []Value // built-in scope; can get from but can't set in

	infoSource InfoSource // provides symbol names and user function information
	trace      []string   // call stack trace

	gil sync.Mutex     // global interpreter lock
	wg  sync.WaitGroup // wait for all threads
}

type CoRoutine struct {
	ip       int      // instruction pointer
	vm       *Machine // the machine that this co-routine is serving
	code     []byte   // the code of the whole program for quicker access
	captured []*Value // captured variables of the current function being executed
	stack    []*Value // data-stack for local variables accessible in the current call stack
	basis    []int    // one base per function; basis[-1] is where the current function's locals start at
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

func (rt *CoRoutine) newRoutine(ip int, locals []*Value, captured []*Value) *CoRoutine {
	return &CoRoutine{ip, rt.vm, rt.code, captured, locals, []int{0}}
}

/* func (rt *Routine) terminate() {
	m.wg.Done()
	ReleaseGIL()
} */

func (rt *CoRoutine) storeLocal(index int, value Value) {
	*(rt.stack[rt.getCurrentBase()+index]) = value
}

func (rt *CoRoutine) storeCaptured(index int, value Value) {
	*(rt.captured[index]) = value
}

func (rt *CoRoutine) getLocal(index int) Value {
	return *(rt.stack[rt.getCurrentBase()+index])
}

func (rt *CoRoutine) capture(index int, scroll int) *Value {
	return rt.stack[rt.getScrolledBase(scroll)+index]
}

func (rt *CoRoutine) getCaptured(index int) Value {
	return *(rt.captured[index])
}

func (rt *CoRoutine) getCurrentBase() int {
	return rt.basis[len(rt.basis)-1]
}

func (rt *CoRoutine) getScrolledBase(scroll int) int {
	return rt.basis[len(rt.basis)-scroll]
}

func (rt *CoRoutine) pushBase(base int) {
	rt.basis = append(rt.basis, base)
}

func (rt *CoRoutine) popBase() {
	rt.basis = rt.basis[:len(rt.basis)-1]
}

func (rt *CoRoutine) popLocals(n int) {
	rt.stack = rt.stack[:len(rt.stack)-n]
}

// enclosing scope & func combo
type UserFn struct {
	vm       *Machine
	captured []*Value
	info     *FuncInfo
}

func (fn UserFn) NumArgs() int {
	return len(fn.info.Args)
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
