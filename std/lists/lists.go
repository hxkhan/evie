package lists

import (
	"strings"

	"github.com/hxkhan/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage()
	pkg.SetSymbol("join", join)
	return pkg
}

var join = vm.BoxGoFunc(&vm.GoFunc{
	Name:      "join",
	Arguments: 2,
	IsMethod:  false,
	Fn: func(fbr *vm.Fiber) (vm.Value, vm.Exception) {
		parts, ok1 := fbr.GetLocal(0).AsArray()
		sep, ok2 := fbr.GetLocal(1).AsString()

		if ok1 && ok2 {
			parts.MU.RLock()
			defer parts.MU.RUnlock()

			strs := make([]string, len(parts.Data))
			for i, part := range parts.Data {
				str, ok := part.AsString()
				if !ok {
					return vm.Value{}, vm.ErrTypes
				}
				strs[i] = str
			}

			return vm.BoxString(strings.Join(strs, sep)), nil
		}

		return vm.Value{}, vm.ErrTypes
	},
})
