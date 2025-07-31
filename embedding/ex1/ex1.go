package main

import (
	"fmt"

	"github.com/hxkhan/evie/std/fs"
	"github.com/hxkhan/evie/std/json"
	"github.com/hxkhan/evie/std/time"
	"github.com/hxkhan/evie/vm"
)

func main() {
	// universal-statics
	statics := map[string]vm.Value{
		"pi":     vm.BoxFloat64(3.14159),
		"time":   time.Construct().Box(),
		"before": vm.BoxString("Hello World"),
	}

	// package-statics for packages that import them via the header
	// e.g. package foo imports("bar")
	resolver := func(name string) vm.Package {
		switch name {
		case "fs":
			return fs.Construct()
		case "json":
			return json.Construct()
		}
		panic(fmt.Errorf("constructor not found for '%v'", name))
	}

	// create a vm with our options
	evm := vm.New(vm.Options{
		UniversalStatics: statics,
		ImportResolver:   resolver,
	})

	// evaluate our script
	result, err := evm.EvalScript([]byte(
		`package main imports("fs")
		
		fn main() {
            after := "Bye World"

            echo before
            await time.timer(pi * 1000)
            echo after

            return await fs.readFile("file.txt")
		}`,
	))

	// check errors
	if err != nil {
		panic(err)
	}

	// print the result (nothing in this case)
	if result.IsTruthy() {
		fmt.Println(result)
	}

	// get a reference to the main package
	pkgMain := evm.GetPackage("main")
	if pkgMain == nil {
		panic("package main not found")
	}

	// get a reference to the main symbol
	symMain, exists := pkgMain.GetSymbol("main")
	if !exists {
		panic("symbol fib not found")
	}

	// type assert it
	main, ok := symMain.AsUserFn()
	if !ok {
		panic("symbol fib is not a function")
	}

	// call it & check for errors
	result, err = main.Call()
	if err != nil {
		panic(err)
	}

	// print the result
	if !result.IsNull() {
		if buffer, ok := result.AsBuffer(); ok {
			fmt.Println(string(buffer))
		}
	}
}
