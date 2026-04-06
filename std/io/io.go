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
	Name:      "print",
	Arguments: 1,
	IsMethod:  false,
	Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		fmt.Print(fbr.GetLocal(0))
		return vm.Value{}, nil
	},
})

var println = vm.BoxGoFunc(&vm.GoFunc{
	Name:      "println",
	Arguments: 1,
	IsMethod:  false,
	Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		fmt.Println(fbr.GetLocal(0))
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
