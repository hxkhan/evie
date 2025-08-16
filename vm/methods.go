package vm

import (
	"strings"

	"github.com/hxkhan/evie/vm/fields"
)

var stringMethods = map[fields.ID]*Value{
	fields.Get("split"): BoxGoFunc(func(this, sep Value) (Value, *Exception) {
		if str, ok := this.AsString(); ok {
			if sep, ok := sep.AsString(); ok {

				parts := strings.Split(str, sep)
				result := make([]Value, len(parts))
				for i, part := range parts {
					result[i] = BoxString(part)
				}
				return BoxArray(result), nil
			}
		}
		return Value{}, ErrTypes
	}).Allocate(),
}

var arrayMethods = map[fields.ID]*Value{
	fields.Get("join"): BoxGoFunc(func(this, sep Value) (Value, *Exception) {
		if parts, ok := this.AsArray(); ok {
			if sep, ok := sep.AsString(); ok {

				strs := make([]string, len(parts))
				for i, part := range parts {
					str, ok := part.AsString()
					if !ok {
						return Value{}, ErrTypes
					}
					strs[i] = str
				}

				return BoxString(strings.Join(strs, sep)), nil
			}
		}
		return Value{}, ErrTypes
	}).Allocate(),
}
