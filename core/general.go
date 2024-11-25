package core

import (
	"fmt"
	"strings"
	"sync"

	box "github.com/hk-32/evie/box"
)

func NewProgram(src []byte, entry int, builtins []box.Value, nGlobals int, refs map[int]string, funcInfo map[int]*FuncInfo) (*Routine, error) {
	m := &machine{
		entry:      entry,
		builtins:   builtins,
		references: refs,
		funcs:      funcInfo,
	}

	rt := &Routine{m: m, code: src}

	for range nGlobals {
		rt.active = append(rt.active, new(box.Value))
	}

	rt.pushBase(0)

	/* if observe {
		m.observe(nil, nil)
	} */

	return rt, nil
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
	Capacity    int         // required scope-capacity
	Start       int         // entry point index
	End         int         // associated op.END index
}

type machine struct {
	entry int // main entry point index

	builtins []box.Value // built-in scope; can get from but can't set in

	references map[int]string    // maps references ip's to their names
	funcs      map[int]*FuncInfo // generated function information
	trace      []string          // call stack trace

	gil sync.Mutex     // global interpreter lock
	wg  sync.WaitGroup // wait for all threads
}

type Routine struct {
	m        *machine     // shared machine
	code     []byte       // executable bytes
	ip       int          // instruction pointer
	active   []*box.Value // active variables for all the functions in the call stack
	basis    []int        // one base per function; locals[basis[len(basis)-1]] is where the current function's locals start at
	captured []*box.Value // currently captured variables
}

func (rt *Routine) releaseGIL() {
	rt.m.gil.Unlock()
}

func (rt *Routine) acquireGIL() {
	rt.m.gil.Lock()
}

func (rt *Routine) newRoutine(ip int, locals []*box.Value, captured []*box.Value) *Routine {
	rt.m.wg.Add(1)
	return &Routine{rt.m, rt.code, ip, locals, []int{0}, captured}
}

func (rt *Routine) terminate() {
	rt.m.wg.Done()
	rt.releaseGIL()
}

func (rt *Routine) storeLocal(index int, value box.Value) {
	*(rt.active[rt.getCurrentBase()+index]) = value
}

func (rt *Routine) storeCaptured(index int, value box.Value) {
	*(rt.captured[index]) = value
}

func (rt *Routine) getLocal(index int) box.Value {
	return *(rt.active[rt.getCurrentBase()+index])
}

func (rt *Routine) getCaptured(index int) box.Value {
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
type fn struct {
	captured []*box.Value
	*FuncInfo
}

func (fn fn) String() string {
	return "<function>"
}

/*
	IDEA: instead of (chan TaskResponse) we can have

	type Task struct {
		channel chan TaskResponse
		err error // last error
	}


*/

/* func NewTask() chan TaskResult {
	return make(chan TaskResult, 1)
} */

type Task chan TaskResult

func NewTask(fn func() (any, error)) (Task, error) {
	task := make(Task, 1)
	go func() {
		res, err := fn()
		task <- TaskResult{res, err}
	}()
	return task, nil
}

type TaskResult struct {
	Value any
	Error error
}

// Safely exit when the consumer choses to discontinue the task
/* func SafeExit() {
	if r := recover(); r != nil {
		if r.(error).Error() != "send on closed channel" {
			panic(r)
		}
		fmt.Println("task discontinued")
	}
} */

/* func (ft Task) Resolve(value any, err error) {
	ft <- result{value, err}
} */

type Tuple []any

func Stringify(v any) string {
	switch value := v.(type) {
	case nil:
		return "null"
	case Task:
		return "<task>"

	case []byte:
		return "<buffer>"
	case Tuple:
		builder := strings.Builder{}
		builder.WriteByte('(')

		for i, v := range value {
			if str, ok := v.(string); ok {
				builder.WriteByte('\'')
				builder.WriteString(str)
				builder.WriteByte('\'')
			} else {
				builder.WriteString(Stringify(v))
			}

			if i != len(value)-1 {
				builder.WriteString(", ")
			}
		}

		builder.WriteByte(')')
		return builder.String()

	case []any:
		builder := strings.Builder{}
		builder.WriteByte('[')

		for i, v := range value {
			if str, ok := v.(string); ok {
				builder.WriteByte('\'')
				builder.WriteString(str)
				builder.WriteByte('\'')
			} else {
				builder.WriteString(Stringify(v))
			}

			if i != len(value)-1 {
				builder.WriteString(", ")
			}
		}

		builder.WriteByte(']')
		return builder.String()
	}

	return fmt.Sprint(v)
}

func pop[T any](slice *[]T) T {
	v := (*slice)[len(*slice)-1]
	*slice = (*slice)[:len(*slice)-1]
	return v
}

func peek[T any](slice []T) T {
	return slice[len(slice)-1]
}
