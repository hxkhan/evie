package fs

import (
	"os"

	"github.com/hxkhan/evie/vm"
)

func Constructor() map[string]*vm.Value {
	readFile := vm.BoxGoFunc(readFile)

	return map[string]*vm.Value{
		"readFile": &readFile,
	}
}

func readFile(fileName vm.Value) (vm.Value, *vm.Exception) {
	if fileName, ok := fileName.AsString(); ok {
		return vm.NewTask(func() (vm.Value, *vm.Exception) {
			bytes, err := os.ReadFile(fileName)
			if err != nil {
				return vm.Value{}, vm.CustomError(err.Error())
			}

			return vm.BoxBuffer(bytes), nil
		}), nil
	}
	return vm.Value{}, vm.ErrTypes
}
