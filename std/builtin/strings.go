package builtin

import (
	"strings"

	"github.com/hxkhan/evie/std"
	"github.com/hxkhan/evie/vm"
)

func Export() {
	std.ImportFn(split)
	std.ImportFn(join)
}

func split(str, sep vm.Value) (vm.Value, error) {
	if str, ok := str.AsString(); ok {
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

func join(parts, sep vm.Value) (vm.Value, error) {
	if parts, ok := parts.AsArray(); ok {
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
