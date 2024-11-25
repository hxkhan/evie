package rt

import (
	"fmt"
	"sync"
)

var GIL = new(sync.Mutex)

type Int = int32
type F64 = float64

var cStack []entry

type entry struct {
	name string
	line int
}

type Error struct {
	Msg  string
	Desc string
}

func (err Error) Error() string {
	return err.Msg + ": " + err.Desc
}

func NewError(kind string, msg string, a ...any) Error {
	return Error{kind, fmt.Sprintf(msg, a...)}
}

func TypeError(msg string, a ...any) Error {
	return Error{"TypeError", fmt.Sprintf(msg, a...)}
}

func ExitMain() {
	if r := recover(); r != nil {
		if err, isOurErr := r.(Error); isOurErr {
			fmt.Println(err)
			for _, e := range cStack {
				fmt.Printf("\tin '%v' on line %v\n", e.name, e.line)
			}
			return
		}
		// Not ours... panic again!
		panic(r)
	}
}

// prepare the callstack in the event that there is an error
/* var ExitFn = func(name string) {
	if r := recover(); r != nil {
		cStack = append(cStack, entry{name, -1})
		panic(r)
	}
} */

func Enter(name string) func(*int) {
	return func(line *int) {
		if r := recover(); r != nil {
			cStack = append(cStack, entry{name, *line})
			panic(r)
		}
	}
}

func ExitFn(name string, line *int) {
	if r := recover(); r != nil {
		cStack = append(cStack, entry{name, *line})
		panic(r)
	}
}

/*
func NewLiner() *int {
	line := 0
	return &line, func() {
		if r := recover(); r != nil {
			cStack = append(cStack, name)
			panic(r)
		}
	}
} */

func Catch(callback func(error) any, result *any) {
	if r := recover(); r != nil {
		if err, isOurErr := r.(Error); isOurErr {
			*result = callback(err)
		}
	}
}
