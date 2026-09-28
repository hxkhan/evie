package fs

import (
	"os"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the fs package
func Construct() *vm.Package {
	pkg := vm.NewPackage("fs")
	pkg.Func("readFile", 1, func(c vm.Call) (vm.Value, error) {
		name, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return vm.Value{}, err
		}
		return vm.BoxBuffer(data), nil
	}).Blocking = true
	return pkg
}
