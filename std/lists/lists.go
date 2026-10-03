package lists

import (
	"fmt"
	"strings"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the lists package
func Construct() *vm.Package {
	pkg := vm.NewPackage("lists")
	pkg.Func("join", 2, func(c vm.Call) (vm.Value, error) {
		arr, err := c.Array(0)
		if err != nil {
			return vm.Value{}, err
		}
		sep, err := c.String(1)
		if err != nil {
			return vm.Value{}, err
		}
		parts := make([]string, len(arr.Data))
		for i, item := range arr.Data {
			str, ok := item.AsString()
			if !ok {
				return vm.Value{}, fmt.Errorf("join: element %d is %s, not a string", i, item.TypeOf())
			}
			parts[i] = str
		}
		return vm.BoxString(strings.Join(parts, sep)), nil
	})
	return pkg
}
