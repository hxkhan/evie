package lists

import (
	"strings"

	"hxkhan.dev/evie/vm"
)

func Construct() vm.Package {
	pkg := vm.NewHostPackage("list")
	pkg.SetSymbol("join", vm.BoxGoFunc(join))
	return pkg
}

func join(this, sep vm.Value) (vm.Value, *vm.Exception) {
	if parts, ok := this.AsArray(); ok {
		if sep, ok := sep.AsString(); ok {

			strs := make([]string, len(parts))
			for i, part := range parts {
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
