package main

import (
	"fmt"

	"hxkhan.dev/evie/std/io"
	"hxkhan.dev/evie/std/time"
	"hxkhan.dev/evie/vm"
)

func main() {
	// universal-statics
	statics := map[string]*vm.Value{
		"pi":   vm.BoxNumber(3.14159).Allocate(),  // constant value
		"time": time.Construct().Box().Allocate(), // already instantiated package
	}

	// package-statics for packages that import them via the header
	// e.g. package foo imports("bar")
	resolver := func(name string) vm.Package {
		switch name {
		case "io":
			return io.Construct()
		}
		panic(fmt.Errorf("constructor not found for '%v'", name))
	}

	// create a vm with our options
	evm := vm.New(vm.Options{
		UniversalStatics: statics,
		ImportsResolver:  resolver,
	})

	// evaluate our script
	result, err := evm.EvalScript([]byte(
		`package main imports("io")
		
		fn add(a, b) {
            io.println("add called in the evie world")

			io.println("about to start awaiting PI seconds")
            time.wait(pi * 1000) // wait 3.14 seconds
            return a + b
		}`,
	))

	// check errors
	if err != nil {
		panic(err)
	}

	// print the result (nothing in this case)
	if !result.IsNil() {
		fmt.Println(result)
	}

	// get a reference to the main package
	pkgMain := evm.GetPackage("main")
	if pkgMain == nil {
		panic("package main not found")
	}

	// get a reference to the main symbol
	symAdd, exists := pkgMain.GetSymbol("add")
	if !exists {
		panic("symbol fib not found")
	}

	// type assert it
	add, ok := symAdd.AsUserFn()
	if !ok {
		panic("symbol fib is not a function")
	}

	// call it & check for errors
	result, err = add.Call(vm.BoxNumber(3), vm.BoxNumber(2))
	if err != nil {
		panic(err)
	}

	// print the result
	if !result.IsNil() {
		fmt.Println(result)
	}
}
