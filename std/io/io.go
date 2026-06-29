package io

import (
	"fmt"

	"github.com/hxkhan/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage()
	pkg.SetSymbol("print", print)
	pkg.SetSymbol("println", println)
	pkg.SetSymbol("prompt", prompt)
	pkg.SetSymbol("readln", readln)
	pkg.SetSymbol("dec", dec)
	return pkg
}

var print = vm.BoxGoFunc(&vm.GoFunc{
	Name:       "print",
	Arguments:  0,
	IsVariadic: true,
	IsMethod:   false,
	Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		count := fbr.ArgCount()
		for i := 0; i < count; i++ {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(fbr.GetLocal(int16(i)))
		}
		return vm.Value{}, nil
	},
})

var println = vm.BoxGoFunc(&vm.GoFunc{
	Name:       "println",
	Arguments:  0,
	IsVariadic: true,
	IsMethod:   false,
	Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		count := fbr.ArgCount()
		for i := 0; i < count; i++ {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(fbr.GetLocal(int16(i)))
		}
		fmt.Println()
		return vm.Value{}, nil
	},
})

var prompt = vm.BoxGoFunc(&vm.GoFunc{
	Name:      "prompt",
	Arguments: 1,
	IsMethod:  false,
	Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		fmt.Print(fbr.GetLocal(0))

		var input string
		fmt.Scanln(&input)
		return vm.BoxString(input), nil
	},
})

var readln = vm.BoxGoFunc(&vm.GoFunc{
	Name:      "readln",
	Arguments: 0,
	IsMethod:  false,
	Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		var input string
		fmt.Scanln(&input)
		return vm.BoxString(input), nil
	},
})

var dec = vm.BoxGoFunc(&vm.GoFunc{
	Name:      "dec",
	Arguments: 1,
	IsMethod:  false,
	Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		n := fbr.GetLocal(0)
		if f64, ok := n.AsFloat64(); ok {
			return vm.BoxNumber(f64 - 1), nil
		}
		return vm.Value{}, vm.ErrTypes
	},
})
