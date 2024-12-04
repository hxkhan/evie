package core

import (
	"sync"
)

func SetProgram(code []byte, globals map[string]*Value, builtins []Value, globalScope []*Value, refs map[int]string, funcInfo map[int]*FuncInfo) (*Routine, error) {
	m = machine{
		code:       code,
		globals:    globals,
		boxes:      make(pool[Value], 0, 48), // 48 = pool size of boxes
		builtins:   builtins,
		references: refs,
		funcs:      funcInfo,
	}

	// return main goroutine
	return &Routine{active: globalScope}, nil
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

var m machine

type machine struct {
	code    []byte // executable bytes
	globals map[string]*Value
	boxes   pool[Value]

	builtins []Value // built-in scope; can get from but can't set in

	references map[int]string    // maps references ip's to their names
	funcs      map[int]*FuncInfo // generated function information
	trace      []string          // call stack trace

	gil sync.Mutex     // global interpreter lock
	wg  sync.WaitGroup // wait for all threads
}

type Routine struct {
	ip       int      // instruction pointer
	active   []*Value // active variables for all the functions in the call stack
	basis    []int    // one base per function; locals[basis[len(basis)-1]] is where the current function's locals start at
	captured []*Value // currently captured variables
}

func GetGlobal(name string) *Value {
	return m.globals[name]
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

func (rt *Routine) newRoutine(ip int, locals []*Value, captured []*Value) *Routine {
	return &Routine{ip, locals, []int{0}, captured}
}

/* func (rt *Routine) terminate() {
	m.wg.Done()
	ReleaseGIL()
} */

func (rt *Routine) storeLocal(index int, value Value) {
	*(rt.active[rt.getCurrentBase()+index]) = value
}

func (rt *Routine) storeCaptured(index int, value Value) {
	*(rt.captured[index]) = value
}

func (rt *Routine) getLocal(index int) Value {
	return *(rt.active[rt.getCurrentBase()+index])
}

func (rt *Routine) getCaptured(index int) Value {
	return *(rt.captured[index])
}

func (rt *Routine) getCurrentBase() int {
	return rt.basis[len(rt.basis)-1]
}

func (rt *Routine) getScrolledBase(scroll int) int {
	return rt.basis[len(rt.basis)-scroll]
}

func (rt *Routine) pushBase(base int) {
	rt.basis = append(rt.basis, base)
}

func (rt *Routine) popBase() {
	rt.basis = rt.basis[:len(rt.basis)-1]
}

func (rt *Routine) popLocals(n int) {
	rt.active = rt.active[:len(rt.active)-n]
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
