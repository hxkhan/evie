package builtin

import (
	"strings"

	"github.com/hk-32/evie/core"
	"github.com/hk-32/evie/std"
)

func Export() {
	std.ImportFn(split)
	std.ImportFn(join)
}

func split(str, sep core.Value) (core.Value, error) {
	if str, ok := str.AsString(); ok {
		if sep, ok := sep.AsString(); ok {

			parts := strings.Split(str, sep)
			result := make([]core.Value, len(parts))
			for i, part := range parts {
				result[i] = core.BoxString(part)
			}
			return core.BoxArray(result), nil
		}
	}
	return core.Value{}, core.ErrTypes
}

func join(parts, sep core.Value) (core.Value, error) {
	if parts, ok := parts.AsArray(); ok {
		if sep, ok := sep.AsString(); ok {

			strs := make([]string, len(parts))
			for i, part := range parts {
				str, ok := part.AsString()
				if !ok {
					return core.Value{}, core.ErrTypes
				}
				strs[i] = str
			}

			return core.BoxString(strings.Join(strs, sep)), nil
		}
	}
	return core.Value{}, core.ErrTypes
}
