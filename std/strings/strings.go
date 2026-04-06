package strings

import (
	"strings"

	"github.com/hxkhan/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage()
	pkg.SetSymbol("split", split)
	return pkg
}

var split = vm.BoxGoFunc(&vm.GoFunc{
	Name:      "split",
	Arguments: 2,
	IsMethod:  false,
	Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		str, ok1 := fbr.GetLocal(0).AsString()
		sep, ok2 := fbr.GetLocal(1).AsString()

		if ok1 && ok2 {
			parts := strings.Split(str, sep)
			result := make([]vm.Value, len(parts))
			for i, part := range parts {
				result[i] = vm.BoxString(part)
			}
			return vm.BoxArray(vm.NewArray(result...)), nil
		}

		return vm.Value{}, vm.ErrTypes
	},
})
