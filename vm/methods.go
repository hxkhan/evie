package vm

import (
	"strings"

	"github.com/hxkhan/evie/vm/fields"
)

var stringMethods = map[fields.ID]*Value{
	fields.Get("len"): BoxGoFunc(func(this Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			return BoxNumber(float64(len(this))), nil
		}
		return Value{}, ErrTypes
	}).Allocate(),

	fields.Get("trim"): BoxGoFunc(func(this Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			return BoxString(strings.TrimSpace(this)), nil
		}
		return Value{}, ErrTypes
	}).Allocate(),

	fields.Get("toLower"): BoxGoFunc(func(this Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			return BoxString(strings.ToLower(this)), nil
		}
		return Value{}, ErrTypes
	}).Allocate(),

	fields.Get("toUpper"): BoxGoFunc(func(this Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			return BoxString(strings.ToUpper(this)), nil
		}
		return Value{}, ErrTypes
	}).Allocate(),

	fields.Get("contains"): BoxGoFunc(func(this, substr Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			if substr, ok := substr.AsString(); ok {
				return BoxBool(strings.Contains(this, substr)), nil
			}
		}
		return Value{}, ErrTypes
	}).Allocate(),

	fields.Get("startsWith"): BoxGoFunc(func(this, prefix Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			if prefix, ok := prefix.AsString(); ok {
				return BoxBool(strings.HasPrefix(this, prefix)), nil
			}
		}
		return Value{}, ErrTypes
	}).Allocate(),

	fields.Get("endsWith"): BoxGoFunc(func(this, suffix Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			if suffix, ok := suffix.AsString(); ok {
				return BoxBool(strings.HasSuffix(this, suffix)), nil
			}
		}
		return Value{}, ErrTypes
	}).Allocate(),

	fields.Get("split"): BoxGoFunc(func(this, sep Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			if sep, ok := sep.AsString(); ok {

				parts := strings.Split(this, sep)
				result := make([]Value, len(parts))
				for i, part := range parts {
					result[i] = BoxString(part)
				}
				return BoxArray(NewArray(result...)), nil
			}
		}
		return Value{}, ErrTypes
	}).Allocate(),
}

var arrayMethods = map[fields.ID]*Value{
	fields.Get("join"): BoxGoFunc(func(this, sep Value) (Value, Exception) {
		if parts, ok := this.AsArray(); ok {
			if sep, ok := sep.AsString(); ok {
				parts.MU.RLock()
				defer parts.MU.RUnlock()

				strs := make([]string, len(parts.Data))
				for i, part := range parts.Data {
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
