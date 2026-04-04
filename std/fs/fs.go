package fs

import (
	"os"

	"github.com/hxkhan/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage()
	pkg.SetSymbol("readFile", vm.BoxGoFunc(readFile))
	return pkg
}

func readFile(fileName vm.Value) (vm.Value, vm.Exception) {
	if fileName, ok := fileName.AsString(); ok {
		bytes, err := os.ReadFile(fileName)
		if err != nil {
			return vm.Value{}, vm.CustomError(err.Error())
		}

		return vm.BoxBuffer(bytes), nil
	}
	return vm.Value{}, vm.ErrTypes
}
