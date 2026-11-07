package vm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hxkhan/evie/vm/fields"
)

// builtinType is a type known to the runtime: it constructs values when called
// and holds the methods of the values it describes
type builtinType struct {
	Name        string
	Constructor *GoFunc
	Fields      map[fields.ID]Value
}

func newBuiltinType(name string, constructor *GoFunc, fields map[fields.ID]Value) *builtinType {
	return &builtinType{
		Name:        name,
		Constructor: constructor,
		Fields:      fields,
	}
}

var builtins = map[string]*Value{
	"string": boxBuiltinType(typeString).allocate(),
	"error":  boxBuiltinType(typeError).allocate(),
	"array":  boxBuiltinType(typeArray).allocate(),
	"object": boxBuiltinType(typeObject).allocate(),
	"buffer": boxBuiltinType(typeBuffer).allocate(),
	"task":   boxBuiltinType(typeTask).allocate(),
	"number": boxBuiltinType(typeNumber).allocate(),
	"bool":   boxBuiltinType(typeBool).allocate(),
}

// receivers of builtin methods are always of the right kind: the runtime only
// binds a method to a value of its own type, so these conversions cannot fail
func (x Value) mustError() *Error      { e, _ := x.AsError(); return e }
func (x Value) mustString() string     { s, _ := x.AsString(); return s }
func (x Value) mustArray() *Array      { a, _ := x.AsArray(); return a }
func (x Value) mustBuffer() []byte     { b, _ := x.AsBuffer(); return b }
func (x Value) mustBufferPtr() *[]byte { return (*[]byte)(x.pointer) }
func (x Value) mustTask() *Task        { t, _ := x.AsTask(); return t }
func (x Value) mustObject() map[string]Value {
	o, _ := x.AsObject()
	return o
}

var typeNumber = newBuiltinType("number", NewFunc("number", 1, func(c Call) (Value, error) {
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

var typeBool = newBuiltinType("bool", NewFunc("bool", 1, func(c Call) (Value, error) {
	return BoxBool(c.Arg(0).IsTruthy()), nil
}), methodTable())

var typeError = newBuiltinType("error", NewFunc("error", 1, func(c Call) (Value, error) {
	return boxError(customError("%s", c.Arg(0).String())), nil
}), methodTable(
	newMethod("message", 0, func(c Call) (Value, error) {
		return BoxString(c.This().mustError().Message), nil
	}),
	newMethod("string", 0, func(c Call) (Value, error) {
		return BoxString(c.This().mustError().Message), nil
	}),
))

var typeString = newBuiltinType("string", NewFunc("string", 1, func(c Call) (Value, error) {
	arg := c.Arg(0)
	if buf, ok := arg.AsBuffer(); ok {
		return BoxString(string(buf)), nil
	}
	return BoxString(arg.String()), nil
}), methodTable(
	newMethod("len", 0, func(c Call) (Value, error) {
		return BoxNumber(float64(len(c.This().mustString()))), nil
	}),
	newMethod("trim", 0, func(c Call) (Value, error) {
		return BoxString(strings.TrimSpace(c.This().mustString())), nil
	}),
	newMethod("toLower", 0, func(c Call) (Value, error) {
		return BoxString(strings.ToLower(c.This().mustString())), nil
	}),
	newMethod("toUpper", 0, func(c Call) (Value, error) {
		return BoxString(strings.ToUpper(c.This().mustString())), nil
	}),
	newMethod("contains", 1, func(c Call) (Value, error) {
		sub, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		return BoxBool(strings.Contains(c.This().mustString(), sub)), nil
	}),
	newMethod("startsWith", 1, func(c Call) (Value, error) {
		prefix, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		return BoxBool(strings.HasPrefix(c.This().mustString(), prefix)), nil
	}),
	newMethod("endsWith", 1, func(c Call) (Value, error) {
		suffix, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		return BoxBool(strings.HasSuffix(c.This().mustString(), suffix)), nil
	}),
	newMethod("split", 1, func(c Call) (Value, error) {
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
	newVariadicMethod("slice", 1, func(c Call) (Value, error) {
		str := c.This().mustString()
		s, e, err := sliceArgs(c, len(str))
		if err != nil {
			return Value{}, err
		}
		return BoxString(str[s:e]), nil
	}),
))

func sliceArgs(c Call, n int) (int, int, error) {
	if c.Len() > 3 {
		return 0, 0, fmt.Errorf("slice takes 1 or 2 arguments, got %d", c.Len()-1)
	}
	start, err := c.Int(1)
	if err != nil {
		return 0, 0, err
	}
	end := n
	if c.Len() == 3 {
		end, err = c.Int(2)
		if err != nil {
			return 0, 0, err
		}
	}
	s, e := clampSlice(n, start, end)
	return s, e, nil
}

var arrayPush = newMethod("push", 1, func(c Call) (Value, error) {
	arr := c.This().mustArray()
	arr.Data = append(arr.Data, c.Arg(1))
	return c.This(), nil
})

var typeArray = newBuiltinType("array", NewFunc("array", 0, func(c Call) (Value, error) {
	return BoxArray(NewArray()), nil
}), methodTable(
	newMethod("len", 0, func(c Call) (Value, error) {
		return BoxNumber(float64(len(c.This().mustArray().Data))), nil
	}),
	arrayPush,
	alias("append", arrayPush),
	newMethod("pop", 0, func(c Call) (Value, error) {
		arr := c.This().mustArray()
		if len(arr.Data) == 0 {
			return Value{}, nil
		}
		last := len(arr.Data) - 1
		val := arr.Data[last]
		arr.Data = arr.Data[:last]
		return val, nil
	}),
	newVariadicMethod("slice", 1, func(c Call) (Value, error) {
		data := c.This().mustArray().Data
		s, e, err := sliceArgs(c, len(data))
		if err != nil {
			return Value{}, err
		}
		sub := make([]Value, e-s)
		copy(sub, data[s:e])
		return BoxArray(NewArray(sub...)), nil
	}),
	newMethod("filter", 1, func(c Call) (Value, error) {
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
	newMethod("map", 1, func(c Call) (Value, error) {
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
	newMethod("contains", 1, func(c Call) (Value, error) {
		target := c.Arg(1)
		for _, item := range c.This().mustArray().Data {
			if item.Equals(target) {
				return BoxBool(true), nil
			}
		}
		return BoxBool(false), nil
	}),
	newMethod("reverse", 0, func(c Call) (Value, error) {
		data := c.This().mustArray().Data
		reversed := make([]Value, len(data))
		for i, v := range data {
			reversed[len(data)-1-i] = v
		}
		return BoxArray(NewArray(reversed...)), nil
	}),
	newMethod("join", 1, func(c Call) (Value, error) {
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

var objectHas = newMethod("has", 1, func(c Call) (Value, error) {
	key, ok := c.Arg(1).AsString()
	if !ok {
		return BoxBool(false), nil
	}
	_, exists := c.This().mustObject()[key]
	return BoxBool(exists), nil
})

var objectRemove = newMethod("remove", 1, func(c Call) (Value, error) {
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

var typeObject = newBuiltinType("object", NewFunc("object", 0, func(c Call) (Value, error) {
	return BoxObject(make(map[string]Value)), nil
}), methodTable(
	newMethod("keys", 0, func(c Call) (Value, error) {
		keys := sortedKeys(c.This().mustObject())
		arr := make([]Value, len(keys))
		for i, k := range keys {
			arr[i] = BoxString(k)
		}
		return BoxArray(NewArray(arr...)), nil
	}),
	newMethod("values", 0, func(c Call) (Value, error) {
		obj := c.This().mustObject()
		keys := sortedKeys(obj)
		arr := make([]Value, len(keys))
		for i, k := range keys {
			arr[i] = obj[k]
		}
		return BoxArray(NewArray(arr...)), nil
	}),
	newMethod("len", 0, func(c Call) (Value, error) {
		return BoxNumber(float64(len(c.This().mustObject()))), nil
	}),
	objectHas,
	alias("contains", objectHas),
	objectRemove,
	alias("delete", objectRemove),
	newMethod("clear", 0, func(c Call) (Value, error) {
		clear(c.This().mustObject())
		return c.This(), nil
	}),
))

var bufferPush = newMethod("push", 1, func(c Call) (Value, error) {
	bp := c.This().mustBufferPtr()
	arg := c.Arg(1)
	if s, ok := arg.AsString(); ok {
		*bp = append(*bp, s...)
		return c.This(), nil
	}
	if other, ok := arg.AsBuffer(); ok {
		*bp = append(*bp, other...)
		return c.This(), nil
	}
	b, err := c.Int(1)
	if err != nil {
		return Value{}, fmt.Errorf("buffer.push: argument must be a byte (0..255), string or buffer, got %s", arg.TypeOf())
	}
	if b < 0 || b > 255 {
		return Value{}, fmt.Errorf("buffer.push: byte value %d out of range 0..255", b)
	}
	*bp = append(*bp, byte(b))
	return c.This(), nil
})

var typeBuffer = newBuiltinType("buffer", NewVariadic("buffer", 0, func(c Call) (Value, error) {
	if c.Len() == 0 {
		return BoxBuffer([]byte{}), nil
	}
	if c.Len() > 1 {
		return Value{}, fmt.Errorf("buffer takes at most 1 argument, got %d", c.Len())
	}
	arg := c.Arg(0)
	if s, ok := arg.AsString(); ok {
		return BoxBuffer([]byte(s)), nil
	}
	if b, ok := arg.AsBuffer(); ok {
		cp := make([]byte, len(b))
		copy(cp, b)
		return BoxBuffer(cp), nil
	}
	if arr, ok := arg.AsArray(); ok {
		out := make([]byte, len(arr.Data))
		for i, item := range arr.Data {
			f, ok := item.AsFloat64()
			if !ok || float64(int(f)) != f || f < 0 || f > 255 {
				return Value{}, fmt.Errorf("buffer: array element %d must be an integer in 0..255", i)
			}
			out[i] = byte(int(f))
		}
		return BoxBuffer(out), nil
	}
	n, err := c.Int(0)
	if err != nil {
		return Value{}, fmt.Errorf("buffer: argument must be a string, array, buffer or size, got %s", arg.TypeOf())
	}
	if n < 0 {
		return Value{}, fmt.Errorf("buffer: size must not be negative, got %d", n)
	}
	return BoxBuffer(make([]byte, n)), nil
}), methodTable(
	newMethod("string", 0, func(c Call) (Value, error) {
		return BoxString(string(c.This().mustBuffer())), nil
	}),
	newMethod("len", 0, func(c Call) (Value, error) {
		return BoxNumber(float64(len(c.This().mustBuffer()))), nil
	}),
	bufferPush,
	alias("append", bufferPush),
	newVariadicMethod("slice", 1, func(c Call) (Value, error) {
		buf := c.This().mustBuffer()
		s, e, err := sliceArgs(c, len(buf))
		if err != nil {
			return Value{}, err
		}
		sub := make([]byte, e-s)
		copy(sub, buf[s:e])
		return BoxBuffer(sub), nil
	}),
))

var typeTask = newBuiltinType("task", NewFunc("task", 0, func(c Call) (Value, error) {
	return boxTask(newTaskInstance()), nil
}), methodTable(
	newMethod("done", 0, func(c Call) (Value, error) {
		return BoxBool(c.This().mustTask().isDone()), nil
	}),
	newMethod("isDone", 0, func(c Call) (Value, error) {
		return BoxBool(c.This().mustTask().isDone()), nil
	}),
	newMethod("result", 0, func(c Call) (Value, error) {
		res, _ := c.This().mustTask().outcome()
		return res, nil
	}),
	newMethod("error", 0, func(c Call) (Value, error) {
		_, exc := c.This().mustTask().outcome()
		if exc != nil {
			return boxError(exc), nil
		}
		return Value{}, nil
	}),
	newMethod("await", 0, func(c Call) (Value, error) {
		return plain(c.This().mustTask().await(c.fbr))
	}),
	newMethod("cancel", 0, func(c Call) (Value, error) {
		c.This().mustTask().Cancel()
		return c.This(), nil
	}),
))

// alias registers the same native under a second name
func alias(name string, fn *GoFunc) *GoFunc {
	copy := *fn
	copy.Name = name
	return &copy
}
