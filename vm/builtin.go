package vm

import (
	"strings"

	"github.com/hxkhan/evie/vm/fields"
)

type BuiltinType struct {
	Name        string
	Constructor *GoFunc
	Fields      map[fields.ID]Value
}

func NewBuiltinType(name string, constructor *GoFunc, fields map[fields.ID]Value) *BuiltinType {
	return &BuiltinType{
		Name:        name,
		Constructor: constructor,
		Fields:      fields,
	}
}

var builtins = map[string]*Value{
	"string": BoxBuiltinType(typeString).Allocate(),
	"error":  BoxBuiltinType(typeError).Allocate(),
}

var typeError = NewBuiltinType("error", &GoFunc{
	Name: "error",
	Fn: func(fbr *Fiber) (Value, Exception) {
		return BoxString(fbr.GetLocal(0).String()), nil
	},
	Arguments: 1,
}, nil)

var typeString = NewBuiltinType("string", &GoFunc{
	Name: "string",
	Fn: func(fbr *Fiber) (Value, Exception) {
		return BoxString(fbr.GetLocal(0).String()), nil
	},
	Arguments: 1,
}, map[fields.ID]Value{
	fields.Get("len"): BoxGoFunc(&GoFunc{
		Name:      "len",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if this, ok := fbr.GetLocal(0).AsString(); ok {
				return BoxNumber(float64(len(this))), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("trim"): BoxGoFunc(&GoFunc{
		Name:      "trim",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if this, ok := fbr.GetLocal(0).AsString(); ok {
				return BoxString(strings.TrimSpace(this)), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("toLower"): BoxGoFunc(&GoFunc{
		Name:      "toLower",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if this, ok := fbr.GetLocal(0).AsString(); ok {
				return BoxString(strings.ToLower(this)), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("toUpper"): BoxGoFunc(&GoFunc{
		Name:      "toUpper",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if this, ok := fbr.GetLocal(0).AsString(); ok {
				return BoxString(strings.ToUpper(this)), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("contains"): BoxGoFunc(&GoFunc{
		Name:      "contains",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			this, ok1 := fbr.GetLocal(0).AsString()
			substr, ok2 := fbr.GetLocal(1).AsString()
			if ok1 && ok2 {
				return BoxBool(strings.Contains(this, substr)), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("startsWith"): BoxGoFunc(&GoFunc{
		Name:      "startsWith",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			this, ok1 := fbr.GetLocal(0).AsString()
			prefix, ok2 := fbr.GetLocal(1).AsString()
			if ok1 && ok2 {
				return BoxBool(strings.HasPrefix(this, prefix)), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("endsWith"): BoxGoFunc(&GoFunc{
		Name:      "endsWith",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			this, ok1 := fbr.GetLocal(0).AsString()
			suffix, ok2 := fbr.GetLocal(1).AsString()
			if ok1 && ok2 {
				return BoxBool(strings.HasSuffix(this, suffix)), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("split"): BoxGoFunc(&GoFunc{
		Name:      "split",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			this, ok1 := fbr.GetLocal(0).AsString()
			sep, ok2 := fbr.GetLocal(1).AsString()
			if ok1 && ok2 {
				parts := strings.Split(this, sep)
				result := make([]Value, len(parts))
				for i, part := range parts {
					result[i] = BoxString(part)
				}
				return BoxArray(NewArray(result...)), nil
			}
			return Value{}, ErrTypes
		},
	}),
})

var typeArray = NewBuiltinType("array", nil, map[fields.ID]Value{
	fields.Get("join"): BoxGoFunc(&GoFunc{
		Name:      "join",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			parts, ok1 := fbr.GetLocal(0).AsArray()
			sep, ok2 := fbr.GetLocal(1).AsString()

			if ok1 && ok2 {
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
			return Value{}, ErrTypes
		},
	}),

	fields.Get("map"): BoxGoFunc(&GoFunc{
		Name:      "map",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (l Value, e Exception) {
			parts, ok1 := fbr.GetLocal(0).AsArray()
			callback := fbr.GetLocal(1)

			if ok1 {
				parts.MU.RLock()
				defer parts.MU.RUnlock()

				result := make([]Value, len(parts.Data))
				for i, part := range parts.Data {
					v, exc := callback.Call(fbr, part)
					if exc != nil {
						return v, exc
					}
					result[i] = v
				}

				return BoxArray(NewArray(result...)), nil
			}

			return Value{}, ErrTypes
		},
	}),
})
