package vm

import (
	"reflect"
	"strings"
	"unsafe"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/vm/fields"
)

type BuiltinType struct {
	Name        string
	Constructor GoFunc
	Fields      map[fields.ID]Value
}

func NewBuiltinType[T SafeGoFunc](name string, constructor T, fields map[fields.ID]Value) *BuiltinType {
	return &BuiltinType{
		Name: name,
		Constructor: GoFunc{
			nargs: reflect.TypeOf(constructor).NumIn(),
			ptr:   unsafe.Pointer(&constructor),
			mode:  ast.UndefinedMode,
		},
		Fields: fields,
	}
}

var builtins = map[string]*Value{
	"string": BoxBuiltinType(typeString).Allocate(),
	"error":  BoxUserStruct(exception).Allocate(),
}

var typeString = NewBuiltinType("string", func(x Value) (Value, Exception) {
	return BoxString(x.String()), nil
}, map[fields.ID]Value{
	fields.Get("len"): BoxGoMethod(func(this Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			return BoxNumber(float64(len(this))), nil
		}
		return Value{}, ErrTypes
	}),

	fields.Get("trim"): BoxGoMethod(func(this Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			return BoxString(strings.TrimSpace(this)), nil
		}
		return Value{}, ErrTypes
	}),

	fields.Get("toLower"): BoxGoMethod(func(this Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			return BoxString(strings.ToLower(this)), nil
		}
		return Value{}, ErrTypes
	}),

	fields.Get("toUpper"): BoxGoMethod(func(this Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			return BoxString(strings.ToUpper(this)), nil
		}
		return Value{}, ErrTypes
	}),

	fields.Get("contains"): BoxGoMethod(func(this, substr Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			if substr, ok := substr.AsString(); ok {
				return BoxBool(strings.Contains(this, substr)), nil
			}
		}
		return Value{}, ErrTypes
	}),

	fields.Get("startsWith"): BoxGoMethod(func(this, prefix Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			if prefix, ok := prefix.AsString(); ok {
				return BoxBool(strings.HasPrefix(this, prefix)), nil
			}
		}
		return Value{}, ErrTypes
	}),

	fields.Get("endsWith"): BoxGoMethod(func(this, suffix Value) (Value, Exception) {
		if this, ok := this.AsString(); ok {
			if suffix, ok := suffix.AsString(); ok {
				return BoxBool(strings.HasSuffix(this, suffix)), nil
			}
		}
		return Value{}, ErrTypes
	}),

	fields.Get("split"): BoxGoMethod(func(this, sep Value) (Value, Exception) {
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
	}),
})

var typeArray = NewBuiltinType("array", func(x Value) (Value, Exception) {
	return BoxString(x.String()), nil
}, map[fields.ID]Value{
	fields.Get("join"): BoxGoMethod(func(this, sep Value) (Value, Exception) {
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
	}),

	fields.Get("map"): BoxGoMethod(func(this, fn Value) (Value, Exception) {
		if parts, ok := this.AsArray(); ok {
			if fn, ok := fn.AsUserFn(); ok {
				parts.MU.RLock()
				defer parts.MU.RUnlock()

				result := make([]Value, len(parts.Data))
				for i, part := range parts.Data {
					v, exc := fn.Call(part)
					if exc != nil {
						return v, exc.(Exception)
					}

					result[i] = v
				}

				return BoxArray(NewArray(result...)), nil
			}

			/* if fn, ok := fn.AsGoFunc(); ok {
				parts.MU.RLock()
				defer parts.MU.RUnlock()

				result := make([]Value, len(parts.Data))
				for i, part := range parts.Data {
					v, exc := fn.call(part)
					if exc != nil {
						return v, exc.(Exception)
					}

					result[i] = v
				}

				return BoxArray(NewArray(result...)), nil
			} */
		}
		return Value{}, ErrTypes
	}),
})
