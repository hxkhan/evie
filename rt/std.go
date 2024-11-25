package rt

import (
	"fmt"
	"time"
)

/* func SetThrow(thrower func(error) error) {
	throw = thrower
}

// host will set this variable
var throw = func(err error) error {
	panic(err)
} */

var Println = NewFn("println", func(arg any) any {
	n, err := fmt.Println(arg)
	if err != nil {
		panic(err)
	}
	return n
})

var Sleep = NewFn("sleep", func(duration any) any {
	GIL.Unlock()
	time.Sleep(time.Millisecond * time.Duration(duration.(int64)))
	GIL.Lock()
	return nil
})

func fib(n int64) int64 {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

var Fibn = NewFn("fibn", func(arg any) any {
	return fib(arg.(int64))
})
