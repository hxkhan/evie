package vm

import (
	"sort"
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
	"array":  BoxBuiltinType(typeArray).Allocate(),
	"object": BoxBuiltinType(typeObject).Allocate(),
}

var typeError = NewBuiltinType("error", &GoFunc{
	Name: "error",
	Fn: func(fbr *Fiber) (Value, Exception) {
		return BoxError(CustomError("%s", fbr.GetLocal(0).String())), nil
	},
	Arguments: 1,
}, map[fields.ID]Value{
	fields.Get("message"): BoxGoFunc(&GoFunc{
		Name:      "message",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if this, ok := fbr.GetLocal(0).AsError(); ok {
				return BoxString(this.Message), nil
			}
			return Value{}, ErrTypes
		},
	}),
	fields.Get("string"): BoxGoFunc(&GoFunc{
		Name:      "string",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if this, ok := fbr.GetLocal(0).AsError(); ok {
				return BoxString(this.Message), nil
			}
			return Value{}, ErrTypes
		},
	}),
})

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

var arrayPushFunc = BoxGoFunc(&GoFunc{
	Name:      "push",
	Arguments: 2,
	IsMethod:  true,
	Fn: func(fbr *Fiber) (Value, Exception) {
		parts, ok := fbr.GetLocal(0).AsArray()
		val := fbr.GetLocal(1)
		if ok {
			parts.MU.Lock()
			parts.Data = append(parts.Data, val)
			parts.MU.Unlock()
			return BoxArray(parts), nil
		}
		return Value{}, ErrTypes
	},
})

var typeArray = NewBuiltinType("array", &GoFunc{
	Name: "array",
	Fn: func(fbr *Fiber) (Value, Exception) {
		return BoxArray(NewArray()), nil
	},
	Arguments: 0,
}, map[fields.ID]Value{
	fields.Get("len"): BoxGoFunc(&GoFunc{
		Name:      "len",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if parts, ok := fbr.GetLocal(0).AsArray(); ok {
				parts.MU.RLock()
				defer parts.MU.RUnlock()
				return BoxNumber(float64(len(parts.Data))), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("push"):   arrayPushFunc,
	fields.Get("append"): arrayPushFunc,

	fields.Get("pop"): BoxGoFunc(&GoFunc{
		Name:      "pop",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if parts, ok := fbr.GetLocal(0).AsArray(); ok {
				parts.MU.Lock()
				defer parts.MU.Unlock()
				if len(parts.Data) == 0 {
					return Value{}, nil
				}
				lastIdx := len(parts.Data) - 1
				val := parts.Data[lastIdx]
				parts.Data = parts.Data[:lastIdx]
				return val, nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("slice"): BoxGoFunc(&GoFunc{
		Name:      "slice",
		Arguments: 3,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			parts, ok1 := fbr.GetLocal(0).AsArray()
			startF, ok2 := fbr.GetLocal(1).AsFloat64()
			endF, ok3 := fbr.GetLocal(2).AsFloat64()
			if ok1 && ok2 && ok3 {
				parts.MU.RLock()
				defer parts.MU.RUnlock()

				n := len(parts.Data)
				start := int(startF)
				end := int(endF)

				if start < 0 {
					start = n + start
					if start < 0 {
						start = 0
					}
				}
				if end < 0 {
					end = n + end
					if end < 0 {
						end = 0
					}
				}
				if start > n {
					start = n
				}
				if end > n {
					end = n
				}
				if start > end {
					return BoxArray(NewArray()), nil
				}

				sub := make([]Value, end-start)
				copy(sub, parts.Data[start:end])
				return BoxArray(NewArray(sub...)), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("filter"): BoxGoFunc(&GoFunc{
		Name:      "filter",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			parts, ok1 := fbr.GetLocal(0).AsArray()
			callback := fbr.GetLocal(1)
			if ok1 {
				parts.MU.RLock()
				defer parts.MU.RUnlock()

				var result []Value
				for _, part := range parts.Data {
					v, exc := callback.Call(fbr, part)
					if exc != nil {
						return v, exc
					}
					if v.IsTruthy() {
						result = append(result, part)
					}
				}
				return BoxArray(NewArray(result...)), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("contains"): BoxGoFunc(&GoFunc{
		Name:      "contains",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			parts, ok1 := fbr.GetLocal(0).AsArray()
			target := fbr.GetLocal(1)
			if ok1 {
				parts.MU.RLock()
				defer parts.MU.RUnlock()
				for _, item := range parts.Data {
					if item.Equals(target) {
						return BoxBool(true), nil
					}
				}
				return BoxBool(false), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("reverse"): BoxGoFunc(&GoFunc{
		Name:      "reverse",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			parts, ok1 := fbr.GetLocal(0).AsArray()
			if ok1 {
				parts.MU.RLock()
				defer parts.MU.RUnlock()
				n := len(parts.Data)
				reversed := make([]Value, n)
				for i, v := range parts.Data {
					reversed[n-1-i] = v
				}
				return BoxArray(NewArray(reversed...)), nil
			}
			return Value{}, ErrTypes
		},
	}),

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

var typeObject = NewBuiltinType("object", &GoFunc{
	Name: "object",
	Fn: func(fbr *Fiber) (Value, Exception) {
		return BoxObject(make(map[fields.ID]Value)), nil
	},
	Arguments: 0,
}, map[fields.ID]Value{
	fields.Get("keys"): BoxGoFunc(&GoFunc{
		Name:      "keys",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if obj, ok := fbr.GetLocal(0).AsObject(); ok {
				keys := make([]string, 0, len(obj))
				for k := range obj {
					keys = append(keys, fields.Lookup(k))
				}
				sort.Strings(keys)
				arr := make([]Value, len(keys))
				for i, k := range keys {
					arr[i] = BoxString(k)
				}
				return BoxArray(NewArray(arr...)), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("values"): BoxGoFunc(&GoFunc{
		Name:      "values",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if obj, ok := fbr.GetLocal(0).AsObject(); ok {
				keys := make([]string, 0, len(obj))
				for k := range obj {
					keys = append(keys, fields.Lookup(k))
				}
				sort.Strings(keys)
				arr := make([]Value, len(keys))
				for i, k := range keys {
					arr[i] = obj[fields.Get(k)]
				}
				return BoxArray(NewArray(arr...)), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("len"): BoxGoFunc(&GoFunc{
		Name:      "len",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if obj, ok := fbr.GetLocal(0).AsObject(); ok {
				return BoxNumber(float64(len(obj))), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("has"): BoxGoFunc(&GoFunc{
		Name:      "has",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if obj, ok := fbr.GetLocal(0).AsObject(); ok {
				keyStr, ok := fbr.GetLocal(1).AsString()
				if !ok {
					return BoxBool(false), nil
				}
				_, exists := obj[fields.Get(keyStr)]
				return BoxBool(exists), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("contains"): BoxGoFunc(&GoFunc{
		Name:      "contains",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if obj, ok := fbr.GetLocal(0).AsObject(); ok {
				keyStr, ok := fbr.GetLocal(1).AsString()
				if !ok {
					return BoxBool(false), nil
				}
				_, exists := obj[fields.Get(keyStr)]
				return BoxBool(exists), nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("remove"): BoxGoFunc(&GoFunc{
		Name:      "remove",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if obj, ok := fbr.GetLocal(0).AsObject(); ok {
				keyStr, ok := fbr.GetLocal(1).AsString()
				if !ok {
					return Value{}, nil
				}
				fid := fields.Get(keyStr)
				val, exists := obj[fid]
				if exists {
					delete(obj, fid)
					return val, nil
				}
				return Value{}, nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("delete"): BoxGoFunc(&GoFunc{
		Name:      "delete",
		Arguments: 2,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if obj, ok := fbr.GetLocal(0).AsObject(); ok {
				keyStr, ok := fbr.GetLocal(1).AsString()
				if !ok {
					return Value{}, nil
				}
				fid := fields.Get(keyStr)
				val, exists := obj[fid]
				if exists {
					delete(obj, fid)
					return val, nil
				}
				return Value{}, nil
			}
			return Value{}, ErrTypes
		},
	}),

	fields.Get("clear"): BoxGoFunc(&GoFunc{
		Name:      "clear",
		Arguments: 1,
		IsMethod:  true,
		Fn: func(fbr *Fiber) (Value, Exception) {
			if obj, ok := fbr.GetLocal(0).AsObject(); ok {
				for k := range obj {
					delete(obj, k)
				}
				return fbr.GetLocal(0), nil
			}
			return Value{}, ErrTypes
		},
	}),
})

