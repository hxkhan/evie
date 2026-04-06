package fs

import (
	"os"

	"github.com/hxkhan/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage()
	pkg.SetSymbol("readFile", readFile)
	return pkg
}

var readFile = vm.BoxGoFunc(&vm.GoFunc{
	Name:      "readFile",
	Arguments: 1,
	IsMethod:  false,
	Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		if fileName, ok := fbr.GetLocal(0).AsString(); ok {
			bytes, err := os.ReadFile(fileName)
			if err != nil {
				return vm.Value{}, vm.CustomError(err.Error())
			}

			return vm.BoxBuffer(bytes), nil
		}

		return vm.Value{}, vm.ErrTypes
	},
})
