package io

import (
	"fmt"

	"github.com/hxkhan/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage("io")
	pkg.SetSymbol("print", print)
	pkg.SetSymbol("println", println)
	pkg.SetSymbol("input", input)
	pkg.SetSymbol("dec", dec)
	return pkg
}

var print = vm.BoxGoFunc(func(output vm.Value) (vm.Value, *vm.Exception) {
	fmt.Print(output)
	return vm.Value{}, nil
})

var println = vm.BoxGoFunc(func(output vm.Value) (vm.Value, *vm.Exception) {
	fmt.Println(output)
	return vm.Value{}, nil
})

var dec = vm.BoxGoFunc(func(n vm.Value) (vm.Value, *vm.Exception) {
	f64, ok := n.AsFloat64()
	if !ok {
		return vm.Value{}, vm.ErrTypes
	}

	return vm.BoxNumber(f64 - 1), nil
})

var input = vm.BoxGoFunc(func(output vm.Value) (vm.Value, *vm.Exception) {
	fmt.Print(output)
	var input string
	fmt.Scanln(&input)
	return vm.BoxString(input), nil
})
