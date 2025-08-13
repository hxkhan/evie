package strings

import (
	"strings"

	"github.com/hxkhan/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage("string")
	pkg.SetSymbol("split", vm.BoxGoFunc(split))
	return pkg
}

func split(this, sep vm.Value) (vm.Value, *vm.Exception) {
	if str, ok := this.AsString(); ok {
		if sep, ok := sep.AsString(); ok {

			parts := strings.Split(str, sep)
			result := make([]vm.Value, len(parts))
			for i, part := range parts {
				result[i] = vm.BoxString(part)
			}
			return vm.BoxArray(result), nil
		}
	}
	return vm.Value{}, vm.ErrTypes
}
