package lists

import (
	"strings"

	"github.com/hxkhan/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage()
	pkg.SetSymbol("join", vm.BoxGoFunc(join))
	return pkg
}

func join(this, sep vm.Value) (vm.Value, vm.Exception) {
	if parts, ok := this.AsArray(); ok {
		if sep, ok := sep.AsString(); ok {
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
	}
	return vm.Value{}, vm.ErrTypes
}
