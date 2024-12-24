package core

import (
	"sync"
)

var m machine

func Setup(builtins []Value, symbols map[int]string, funcInfo map[int]*FuncInfo) {
	m = machine{
		boxes:      make(pool[Value], 0, 48), // 48 = pool size of boxes
		builtins:   builtins,
		symbolsMap: symbols,
		funcsMap:   funcInfo,
	}
}

func Run(code []byte, globals []*Value) (Value, error) {
	b4 := len(m.code)
	start := 0

	if b4 == 0 {
		m.code = code
	} else {
		// only run the new part
		start = b4
	}

	AcquireGIL()
	defer ReleaseGIL()

	rt := &CoRoutine{stack: globals, basis: []int{0}}

	for rt.ip = start; rt.ip < len(m.code); rt.ip++ {
		// fetch and execute the instruction
		if _, err := instructions[m.code[rt.ip]](rt); err != nil {
			return Value{}, err
		}
	}

	return Value{}, nil
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

type machine struct {
	code  []byte      // executable bytes
	boxes pool[Value] // pool of

	builtins []Value // built-in scope; can get from but can't set in

	symbolsMap map[int]string    // maps references ip's to their names
	funcsMap   map[int]*FuncInfo // generated function information
	trace      []string          // call stack trace

	gil sync.Mutex     // global interpreter lock
	wg  sync.WaitGroup // wait for all threads
}

type CoRoutine struct {
	ip       int      // instruction pointer
	captured []*Value // currently captured variables
	stack    []*Value // local variables for all the functions in the call stack
	basis    []int    // one base per function; basis[-1] is where the current function's locals start at
}

func WaitForNoActivity() {
	m.wg.Wait()
}

func ReleaseGIL() {
	m.gil.Unlock()
}

func AcquireGIL() {
	m.gil.Lock()
}

func (rt *CoRoutine) newRoutine(ip int, locals []*Value, captured []*Value) *CoRoutine {
	return &CoRoutine{ip, captured, locals, []int{0}}
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
	captured []*Value
	*FuncInfo
}

func (fn UserFn) String() string {
	return "<function>"
}

func NewTask(fn func() (Value, error)) Value {
	task := make(chan TaskResult, 1)
	go func() {
		res, err := fn()
		task <- TaskResult{res, err}
	}()
	return BoxTask(task)
}

type TaskResult struct {
	Result Value
	Error  error
}

type Tuple []any

func pop[T any](slice *[]T) T {
	v := (*slice)[len(*slice)-1]
	*slice = (*slice)[:len(*slice)-1]
	return v
}

func peek[T any](slice []T) T {
	return slice[len(slice)-1]
}
