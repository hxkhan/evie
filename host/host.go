package main

import (
	"fmt"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

func main() {
	ip := evie.New(evie.Defaults)

	_, err := ip.EvalScript([]byte(
		`package main
		
		fn fib(n) {
    		if (n < 2) return n
    		return fib(n-1) + fib(n-2)
		}`,
	))

	// Check for errors
	if err != nil {
		panic(err)
	}

	// Get a reference to the global symbol 'fib'
	value := ip.GetGlobal("fib")
	if value == nil {
		panic("fib not found")
	}

	// Type assert the value to a function
	fn, ok := value.AsUserFn()
	if !ok {
		panic("fib is not a function")
	}

	// Call it
	result, err := fn.Call(vm.BoxFloat64(35))
	if err != nil {
		panic(err)
	}

	fmt.Println(result)
}
