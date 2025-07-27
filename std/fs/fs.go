package fs

import (
	"os"

	"github.com/hxkhan/evie/vm"
)

func Instantiate() map[string]*vm.Value {
	readFile := vm.BoxGoFunc(readFile)

	return map[string]*vm.Value{
		"readFile": &readFile,
	}
}

func readFile(fileName vm.Value) (vm.Value, error) {
	if fileName, ok := fileName.AsString(); ok {
		return vm.NewTask(func() (vm.Value, error) {
			bytes, err := os.ReadFile(fileName)
			return vm.BoxBuffer(bytes), err
		}), nil
	}
	return vm.Value{}, vm.ErrTypes
}
