package io

import (
	"fmt"

	"github.com/hxkhan/evie/vm"
)

func Constructor() map[string]*vm.Value {
	return map[string]*vm.Value{
		"print":   &print,
		"println": &println,
		"input":   &input,
	}
}

var print = vm.BoxGoFunc(func(output vm.Value) (vm.Value, *vm.Exception) {
	fmt.Print(output)
	return vm.Value{}, nil
})

var println = vm.BoxGoFunc(func(output vm.Value) (vm.Value, *vm.Exception) {
	fmt.Println(output)
	return vm.Value{}, nil
})

var input = vm.BoxGoFunc(func(output vm.Value) (vm.Value, *vm.Exception) {
	fmt.Print(output)
	var input string
	fmt.Scanln(&input)
	return vm.BoxString(input), nil
})
