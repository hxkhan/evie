package main

import (
	"fmt"

	"github.com/hxkhan/evie"
	"github.com/hxkhan/evie/vm"
)

// The low-level way in: an engine with a prelude the scripts see everywhere,
// a function looked up by path and called with evie values.
func main() {
	prelude := vm.NewPackage("prelude")
	prelude.Set("pi", vm.BoxNumber(3.14159))

	eng := evie.New(evie.Options{Prelude: prelude})

	_, err := eng.Eval(`package main imports("io", "time")

	fn add(a, b) {
		io.println("add called in the evie world")
		io.println("about to wait pi seconds")
		time.wait(pi * 1000)
		return a + b
	}`)
	if err != nil {
		panic(err)
	}

	add, ok := eng.Lookup("main.add")
	if !ok || !add.IsCallable() {
		panic("symbol add not found or not callable")
	}

	result, err := add.Call(vm.BoxNumber(3), vm.BoxNumber(2))
	if err != nil {
		panic(err)
	}
	fmt.Println(result)
}
