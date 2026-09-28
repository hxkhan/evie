package vm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hxkhan/evie/vm/fields"
)

// BuiltinType is a type known to the runtime: it constructs values when called
// and holds the methods of the values it describes
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
	"buffer": BoxBuiltinType(typeBuffer).Allocate(),
	"task":   BoxBuiltinType(typeTask).Allocate(),
	"number": BoxBuiltinType(typeNumber).Allocate(),
	"bool":   BoxBuiltinType(typeBool).Allocate(),
}

// receivers of builtin methods are always of the right kind: the runtime only
// binds a method to a value of its own type, so these conversions cannot fail
func (x Value) mustError() *Error  { e, _ := x.AsError(); return e }
func (x Value) mustString() string { s, _ := x.AsString(); return s }
func (x Value) mustArray() *Array  { a, _ := x.AsArray(); return a }
func (x Value) mustBuffer() []byte { b, _ := x.AsBuffer(); return b }
func (x Value) mustTask() *Task    { t, _ := x.AsTask(); return t }
func (x Value) mustObject() map[string]Value {
	o, _ := x.AsObject()
	return o
}

var typeNumber = NewBuiltinType("number", NewFunc("number", 1, func(c Call) (Value, error) {
	arg := c.Arg(0)
	if _, ok := arg.AsFloat64(); ok {
		return arg, nil
	}
	if b, ok := arg.AsBool(); ok {
		if b {
			return BoxNumber(1), nil
		}
		return BoxNumber(0), nil
	}
	if s, ok := arg.AsString(); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return Value{}, fmt.Errorf("cannot convert '%v' to a number", s)
		}
		return BoxNumber(f), nil
	}
	return Value{}, fmt.Errorf("cannot convert %v to a number", arg.TypeOf())
}), methodTable())

var typeBool = NewBuiltinType("bool", NewFunc("bool", 1, func(c Call) (Value, error) {
	return BoxBool(c.Arg(0).IsTruthy()), nil
}), methodTable())

var typeError = NewBuiltinType("error", NewFunc("error", 1, func(c Call) (Value, error) {
	return BoxError(CustomError("%s", c.Arg(0).String())), nil
}), methodTable(
	NewMethod("message", 0, func(c Call) (Value, error) {
		return BoxString(c.This().mustError().Message), nil
	}),
	NewMethod("string", 0, func(c Call) (Value, error) {
		return BoxString(c.This().mustError().Message), nil
	}),
))

var typeString = NewBuiltinType("string", NewFunc("string", 1, func(c Call) (Value, error) {
	arg := c.Arg(0)
	if buf, ok := arg.AsBuffer(); ok {
		return BoxString(string(buf)), nil
	}
	return BoxString(arg.String()), nil
}), methodTable(
	NewMethod("len", 0, func(c Call) (Value, error) {
		return BoxNumber(float64(len(c.This().mustString()))), nil
	}),
	NewMethod("trim", 0, func(c Call) (Value, error) {
		return BoxString(strings.TrimSpace(c.This().mustString())), nil
	}),
	NewMethod("toLower", 0, func(c Call) (Value, error) {
		return BoxString(strings.ToLower(c.This().mustString())), nil
	}),
	NewMethod("toUpper", 0, func(c Call) (Value, error) {
		return BoxString(strings.ToUpper(c.This().mustString())), nil
	}),
	NewMethod("contains", 1, func(c Call) (Value, error) {
		sub, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		return BoxBool(strings.Contains(c.This().mustString(), sub)), nil
	}),
	NewMethod("startsWith", 1, func(c Call) (Value, error) {
		prefix, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		return BoxBool(strings.HasPrefix(c.This().mustString(), prefix)), nil
	}),
	NewMethod("endsWith", 1, func(c Call) (Value, error) {
		suffix, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		return BoxBool(strings.HasSuffix(c.This().mustString(), suffix)), nil
	}),
	NewMethod("split", 1, func(c Call) (Value, error) {
		sep, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		parts := strings.Split(c.This().mustString(), sep)
		result := make([]Value, len(parts))
		for i, part := range parts {
			result[i] = BoxString(part)
		}
		return BoxArray(NewArray(result...)), nil
	}),
))

var arrayPush = NewMethod("push", 1, func(c Call) (Value, error) {
	arr := c.This().mustArray()
	arr.Data = append(arr.Data, c.Arg(1))
	return c.This(), nil
})

var typeArray = NewBuiltinType("array", NewFunc("array", 0, func(c Call) (Value, error) {
	return BoxArray(NewArray()), nil
}), methodTable(
	NewMethod("len", 0, func(c Call) (Value, error) {
		return BoxNumber(float64(len(c.This().mustArray().Data))), nil
	}),
	arrayPush,
	alias("append", arrayPush),
	NewMethod("pop", 0, func(c Call) (Value, error) {
		arr := c.This().mustArray()
		if len(arr.Data) == 0 {
			return Value{}, nil
		}
		last := len(arr.Data) - 1
		val := arr.Data[last]
		arr.Data = arr.Data[:last]
		return val, nil
	}),
	NewMethod("slice", 2, func(c Call) (Value, error) {
		start, err := c.Int(1)
		if err != nil {
			return Value{}, err
		}
		end, err := c.Int(2)
		if err != nil {
			return Value{}, err
		}
		data := c.This().mustArray().Data
		n := len(data)
		if start < 0 {
			start = max(n+start, 0)
		}
		if end < 0 {
			end = max(n+end, 0)
		}
		start, end = min(start, n), min(end, n)
		if start > end {
			return BoxArray(NewArray()), nil
		}
		sub := make([]Value, end-start)
		copy(sub, data[start:end])
		return BoxArray(NewArray(sub...)), nil
	}),
	NewMethod("filter", 1, func(c Call) (Value, error) {
		callback, err := c.Callable(1)
		if err != nil {
			return Value{}, err
		}
		var result []Value
		for _, item := range c.This().mustArray().Data {
			keep, err := c.Call(callback, item)
			if err != nil {
				return Value{}, err
			}
			if keep.IsTruthy() {
				result = append(result, item)
			}
		}
		return BoxArray(NewArray(result...)), nil
	}),
	NewMethod("map", 1, func(c Call) (Value, error) {
		callback, err := c.Callable(1)
		if err != nil {
			return Value{}, err
		}
		data := c.This().mustArray().Data
		result := make([]Value, len(data))
		for i, item := range data {
			mapped, err := c.Call(callback, item)
			if err != nil {
				return Value{}, err
			}
			result[i] = mapped
		}
		return BoxArray(NewArray(result...)), nil
	}),
	NewMethod("contains", 1, func(c Call) (Value, error) {
		target := c.Arg(1)
		for _, item := range c.This().mustArray().Data {
			if item.Equals(target) {
				return BoxBool(true), nil
			}
		}
		return BoxBool(false), nil
	}),
	NewMethod("reverse", 0, func(c Call) (Value, error) {
		data := c.This().mustArray().Data
		reversed := make([]Value, len(data))
		for i, v := range data {
			reversed[len(data)-1-i] = v
		}
		return BoxArray(NewArray(reversed...)), nil
	}),
	NewMethod("join", 1, func(c Call) (Value, error) {
		sep, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		data := c.This().mustArray().Data
		parts := make([]string, len(data))
		for i, item := range data {
			str, ok := item.AsString()
			if !ok {
				return Value{}, fmt.Errorf("join: element %d is %s, not a string", i, item.TypeOf())
			}
			parts[i] = str
		}
		return BoxString(strings.Join(parts, sep)), nil
	}),
))

var objectHas = NewMethod("has", 1, func(c Call) (Value, error) {
	key, ok := c.Arg(1).AsString()
	if !ok {
		return BoxBool(false), nil
	}
	_, exists := c.This().mustObject()[key]
	return BoxBool(exists), nil
})

var objectRemove = NewMethod("remove", 1, func(c Call) (Value, error) {
	key, ok := c.Arg(1).AsString()
	if !ok {
		return Value{}, nil
	}
	obj := c.This().mustObject()
	val, exists := obj[key]
	if exists {
		delete(obj, key)
		return val, nil
	}
	return Value{}, nil
})

var typeObject = NewBuiltinType("object", NewFunc("object", 0, func(c Call) (Value, error) {
	return BoxObject(make(map[string]Value)), nil
}), methodTable(
	NewMethod("keys", 0, func(c Call) (Value, error) {
		keys := sortedKeys(c.This().mustObject())
		arr := make([]Value, len(keys))
		for i, k := range keys {
			arr[i] = BoxString(k)
		}
		return BoxArray(NewArray(arr...)), nil
	}),
	NewMethod("values", 0, func(c Call) (Value, error) {
		obj := c.This().mustObject()
		keys := sortedKeys(obj)
		arr := make([]Value, len(keys))
		for i, k := range keys {
			arr[i] = obj[k]
		}
		return BoxArray(NewArray(arr...)), nil
	}),
	NewMethod("len", 0, func(c Call) (Value, error) {
		return BoxNumber(float64(len(c.This().mustObject()))), nil
	}),
	objectHas,
	alias("contains", objectHas),
	objectRemove,
	alias("delete", objectRemove),
	NewMethod("clear", 0, func(c Call) (Value, error) {
		clear(c.This().mustObject())
		return c.This(), nil
	}),
))

var typeBuffer = NewBuiltinType("buffer", NewFunc("buffer", 0, func(c Call) (Value, error) {
	return BoxBuffer([]byte{}), nil
}), methodTable(
	NewMethod("string", 0, func(c Call) (Value, error) {
		return BoxString(string(c.This().mustBuffer())), nil
	}),
	NewMethod("len", 0, func(c Call) (Value, error) {
		return BoxNumber(float64(len(c.This().mustBuffer()))), nil
	}),
))

var typeTask = NewBuiltinType("task", NewFunc("task", 0, func(c Call) (Value, error) {
	return BoxTask(NewTaskInstance()), nil
}), methodTable(
	NewMethod("done", 0, func(c Call) (Value, error) {
		return BoxBool(c.This().mustTask().IsDone()), nil
	}),
	NewMethod("isDone", 0, func(c Call) (Value, error) {
		return BoxBool(c.This().mustTask().IsDone()), nil
	}),
	NewMethod("result", 0, func(c Call) (Value, error) {
		res, _ := c.This().mustTask().Result()
		return res, nil
	}),
	NewMethod("error", 0, func(c Call) (Value, error) {
		_, exc := c.This().mustTask().Result()
		if exc != nil {
			return BoxError(exc), nil
		}
		return Value{}, nil
	}),
	NewMethod("await", 0, func(c Call) (Value, error) {
		return plain(c.This().mustTask().Await(c.fbr))
	}),
))

// alias registers the same native under a second name
func alias(name string, fn *GoFunc) *GoFunc {
	copy := *fn
	copy.Name = name
	return &copy
}
