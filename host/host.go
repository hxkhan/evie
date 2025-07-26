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

	// Get a reference to the package
	pkgMain := ip.GetPackage("main")
	if pkgMain == nil {
		panic("no main package found")
	}

	// Get a reference to the 'fib' symbol
	symFib, exists := pkgMain.GetGlobal("fib")
	if !exists {
		panic("symbol fib not found")
	}

	// Type check it
	fib, ok := symFib.AsUserFn()
	if !ok {
		panic("fib is not a function")
	}

	// Call it
	result, err := fib.Call(vm.BoxFloat64(35))
	if err != nil {
		panic(err)
	}

	fmt.Println(result)
}
