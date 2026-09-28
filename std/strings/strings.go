package strings

import (
	"strings"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the strings package
func Construct() *vm.Package {
	pkg := vm.NewPackage("strings")
	pkg.Func("split", 2, func(c vm.Call) (vm.Value, error) {
		str, err := c.String(0)
		if err != nil {
			return vm.Value{}, err
		}
		sep, err := c.String(1)
		if err != nil {
			return vm.Value{}, err
		}
		parts := strings.Split(str, sep)
		result := make([]vm.Value, len(parts))
		for i, part := range parts {
			result[i] = vm.BoxString(part)
		}
		return vm.BoxArray(vm.NewArray(result...)), nil
	})
	return pkg
}
