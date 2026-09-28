package io

import (
	"fmt"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the io package: console output and input
func Construct() *vm.Package {
	pkg := vm.NewPackage("io")
	pkg.Variadic("print", 0, func(c vm.Call) (vm.Value, error) {
		for i := 0; i < c.Len(); i++ {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(c.Arg(i))
		}
		return vm.Value{}, nil
	})
	pkg.Variadic("println", 0, func(c vm.Call) (vm.Value, error) {
		for i := 0; i < c.Len(); i++ {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(c.Arg(i))
		}
		fmt.Println()
		return vm.Value{}, nil
	})
	pkg.Func("prompt", 1, func(c vm.Call) (vm.Value, error) {
		fmt.Print(c.Arg(0))
		var input string
		fmt.Scanln(&input)
		return vm.BoxString(input), nil
	}).Blocking = true
	pkg.Func("readln", 0, func(c vm.Call) (vm.Value, error) {
		var input string
		fmt.Scanln(&input)
		return vm.BoxString(input), nil
	}).Blocking = true
	pkg.Func("dec", 1, func(c vm.Call) (vm.Value, error) {
		n, err := c.Number(0)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxNumber(n - 1), nil
	})
	return pkg
}
