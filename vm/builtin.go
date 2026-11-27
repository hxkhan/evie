package vm

import (
	"cmp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hxkhan/evie/internal/fields"
)

// maxStringAllocBytes caps single-call string expansion in repeat/padStart/padEnd
// and the size of buffer(n), to prevent untrusted scripts from triggering an
// out-of-memory crash.
const maxStringAllocBytes = 64 << 20

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
func (x Value) mustObject() *Object    { return x.object() }

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
			return Value{}, Errorf(KindValue, "cannot convert '%v' to a number", s)
		}
		return BoxNumber(f), nil
	}
	return Value{}, Errorf(KindType, "cannot convert %v to a number", arg.TypeOf())
}), methodTable())

var typeBool = newBuiltinType("bool", NewFunc("bool", 1, func(c Call) (Value, error) {
	return BoxBool(c.Arg(0).IsTruthy()), nil
}), methodTable())

// error(message) makes an error; error(message, fields) also gives it a kind,
// a cause or any other fields, as in error("no config", {kind: "config", path: p})
var typeError = newBuiltinType("error", NewVariadic("error", 1, func(c Call) (Value, error) {
	if c.Len() > 2 {
		return Value{}, Errorf(KindType, "error takes a message and an object of fields, but %d arguments were supplied", c.Len())
	}
	e := customError("%s", c.Arg(0).String())
	if c.Len() == 2 {
		obj, ok := c.Arg(1).AsObject()
		if !ok {
			return Value{}, c.typeError(1, "an object of fields")
		}
		if err := e.take(obj, false); err != nil {
			return Value{}, err
		}
	}
	return boxError(e), nil
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
		return BoxNumber(float64(utf8.RuneCountInString(c.This().mustString()))), nil
	}),
	newMethod("trim", 0, func(c Call) (Value, error) {
		return BoxString(strings.TrimSpace(c.This().mustString())), nil
	}),
	newMethod("trimStart", 0, func(c Call) (Value, error) {
		return BoxString(strings.TrimLeftFunc(c.This().mustString(), unicode.IsSpace)), nil
	}),
	newMethod("trimEnd", 0, func(c Call) (Value, error) {
		return BoxString(strings.TrimRightFunc(c.This().mustString(), unicode.IsSpace)), nil
	}),
	newMethod("trimPrefix", 1, func(c Call) (Value, error) {
		prefix, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		return BoxString(strings.TrimPrefix(c.This().mustString(), prefix)), nil
	}),
	newMethod("trimSuffix", 1, func(c Call) (Value, error) {
		suffix, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		return BoxString(strings.TrimSuffix(c.This().mustString(), suffix)), nil
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
	newMethod("index", 1, func(c Call) (Value, error) {
		sub, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		s := c.This().mustString()
		return BoxNumber(float64(runeIndex(s, strings.Index(s, sub)))), nil
	}),
	newMethod("lastIndex", 1, func(c Call) (Value, error) {
		sub, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		s := c.This().mustString()
		return BoxNumber(float64(runeIndex(s, strings.LastIndex(s, sub)))), nil
	}),
	newMethod("count", 1, func(c Call) (Value, error) {
		sub, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		return BoxNumber(float64(strings.Count(c.This().mustString(), sub))), nil
	}),
	newMethod("replace", 2, func(c Call) (Value, error) {
		oldStr, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		newStr, err := c.String(2)
		if err != nil {
			return Value{}, err
		}
		return BoxString(strings.ReplaceAll(c.This().mustString(), oldStr, newStr)), nil
	}),
	newMethod("replaceFirst", 2, func(c Call) (Value, error) {
		oldStr, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		newStr, err := c.String(2)
		if err != nil {
			return Value{}, err
		}
		return BoxString(strings.Replace(c.This().mustString(), oldStr, newStr, 1)), nil
	}),
	newMethod("repeat", 1, func(c Call) (Value, error) {
		n, err := c.Int(1)
		if err != nil {
			return Value{}, err
		}
		if n < 0 {
			return Value{}, Errorf(KindValue, "repeat: the count must not be negative")
		}
		s := c.This().mustString()
		if len(s) > 0 && n > maxStringAllocBytes/len(s) {
			return Value{}, Errorf(KindValue, "repeat: result exceeds maximum string size (%d bytes)", maxStringAllocBytes)
		}
		return BoxString(strings.Repeat(s, n)), nil
	}),
	newMethod("reverse", 0, func(c Call) (Value, error) {
		runes := []rune(c.This().mustString())
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return BoxString(string(runes)), nil
	}),
	newMethod("padStart", 2, func(c Call) (Value, error) {
		width, err := c.Int(1)
		if err != nil {
			return Value{}, err
		}
		pad, err := c.String(2)
		if err != nil {
			return Value{}, err
		}
		s := c.This().mustString()
		p, err := stringPadding(s, width, pad)
		if err != nil {
			return Value{}, err
		}
		return BoxString(p + s), nil
	}),
	newMethod("padEnd", 2, func(c Call) (Value, error) {
		width, err := c.Int(1)
		if err != nil {
			return Value{}, err
		}
		pad, err := c.String(2)
		if err != nil {
			return Value{}, err
		}
		s := c.This().mustString()
		p, err := stringPadding(s, width, pad)
		if err != nil {
			return Value{}, err
		}
		return BoxString(s + p), nil
	}),
	newMethod("split", 1, func(c Call) (Value, error) {
		sep, err := c.String(1)
		if err != nil {
			return Value{}, err
		}
		return BoxArray(boxStrings(strings.Split(c.This().mustString(), sep))), nil
	}),
	newMethod("fields", 0, func(c Call) (Value, error) {
		return BoxArray(boxStrings(strings.Fields(c.This().mustString()))), nil
	}),
	newMethod("lines", 0, func(c Call) (Value, error) {
		s := c.This().mustString()
		if s == "" {
			return BoxArray(NewArray()), nil
		}
		s = strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
		return BoxArray(boxStrings(strings.Split(s, "\n"))), nil
	}),
	newMethod("chars", 0, func(c Call) (Value, error) {
		s := c.This().mustString()
		chars := make([]Value, 0, len(s))
		for _, r := range s {
			chars = append(chars, BoxString(string(r)))
		}
		return BoxArray(NewArray(chars...)), nil
	}),
	newVariadicMethod("slice", 1, func(c Call) (Value, error) {
		str := c.This().mustString()
		n := utf8.RuneCountInString(str)
		s, e, err := sliceArgs(c, n)
		if err != nil {
			return Value{}, err
		}
		return BoxString(sliceStringRunes(str, n, s, e)), nil
	}),
))

func boxStrings(parts []string) *Array {
	values := make([]Value, len(parts))
	for i, part := range parts {
		values[i] = BoxString(part)
	}
	return NewArray(values...)
}

func runeIndex(s string, byteIndex int) int {
	if byteIndex < 0 {
		return -1
	}
	return utf8.RuneCountInString(s[:byteIndex])
}

func stringPadding(s string, width int, pad string) (string, error) {
	missing := width - utf8.RuneCountInString(s)
	if missing <= 0 || pad == "" {
		return "", nil
	}
	if missing > maxStringAllocBytes/utf8.UTFMax {
		return "", Errorf(KindValue, "pad: target width %d exceeds maximum string size", width)
	}
	padRunes := []rune(pad)
	out := make([]rune, missing)
	for i := range out {
		out[i] = padRunes[i%len(padRunes)]
	}
	return string(out), nil
}

func sliceArgs(c Call, n int) (int, int, error) {
	if c.Len() > 3 {
		return 0, 0, Errorf(KindType, "slice takes 1 or 2 arguments, got %d", c.Len()-1)
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
	arr.push(c.Arg(1))
	return c.This(), nil
})

var arrayIndex = newMethod("index", 1, func(c Call) (Value, error) {
	target := c.Arg(1)
	for i, item := range c.This().mustArray().Data {
		if item.Equals(target) {
			return BoxNumber(float64(i)), nil
		}
	}
	return BoxNumber(-1), nil
})

// findIndex is the position of the first element the callback in argument 1
// is truthy for, or -1
func findIndex(c Call) (int, error) {
	callback, err := c.Callable(1)
	if err != nil {
		return -1, err
	}
	for i, item := range c.This().mustArray().Data {
		ok, err := c.Call(callback, item)
		if err != nil {
			return -1, err
		}
		if ok.IsTruthy() {
			return i, nil
		}
	}
	return -1, nil
}

// sortNatural sorts numbers in ascending order and strings by code point. The
// elements must be all numbers or all strings; anything else needs a function
// that says which of two elements goes first.
func sortNatural(data []Value) error {
	if len(data) == 0 {
		return nil
	}
	if _, ok := data[0].AsFloat64(); ok {
		for i, v := range data {
			if _, ok := v.AsFloat64(); !ok {
				return sortMixed(i, data[0], v)
			}
		}
		slices.SortStableFunc(data, func(a, b Value) int {
			x, _ := a.AsFloat64()
			y, _ := b.AsFloat64()
			return cmp.Compare(x, y)
		})
		return nil
	}
	if _, ok := data[0].AsString(); ok {
		for i, v := range data {
			if _, ok := v.AsString(); !ok {
				return sortMixed(i, data[0], v)
			}
		}
		slices.SortStableFunc(data, func(a, b Value) int {
			return strings.Compare(a.mustString(), b.mustString())
		})
		return nil
	}
	return Errorf(KindType, "sort: cannot order %s without a function; pass one, as in sort(fn(a, b) => a.age < b.age)", article(data[0]))
}

func sortMixed(i int, first, v Value) error {
	return Errorf(KindType, "sort: element %d is %s but element 0 is %s; pass a function that orders them", i, article(v), article(first))
}

// sortBy sorts with the callback in argument 1, which says whether its first
// argument goes before its second. It must return a bool, so that a function
// written to return a number, as JavaScript's sort expects, fails instead of
// silently sorting wrongly.
func sortBy(c Call, data []Value) error {
	callback, err := c.Callable(1)
	if err != nil {
		return err
	}
	var failed error
	sort.SliceStable(data, func(i, j int) bool {
		if failed != nil {
			return false
		}
		before, err := c.Call(callback, data[i], data[j])
		if err != nil {
			failed = err
			return false
		}
		b, ok := before.AsBool()
		if !ok {
			failed = Errorf(KindType, "sort: the function must return a bool saying whether its first argument goes first, but it returned %s", article(before))
			return false
		}
		return b
	})
	return failed
}

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
	arrayIndex,
	alias("indexOf", arrayIndex),
	newMethod("find", 1, func(c Call) (Value, error) {
		i, err := findIndex(c)
		if err != nil || i < 0 {
			return Value{}, err
		}
		return c.This().mustArray().Data[i], nil
	}),
	newMethod("findIndex", 1, func(c Call) (Value, error) {
		i, err := findIndex(c)
		return BoxNumber(float64(i)), err
	}),
	newMethod("some", 1, func(c Call) (Value, error) {
		i, err := findIndex(c)
		return BoxBool(i >= 0), err
	}),
	newMethod("every", 1, func(c Call) (Value, error) {
		callback, err := c.Callable(1)
		if err != nil {
			return Value{}, err
		}
		for _, item := range c.This().mustArray().Data {
			ok, err := c.Call(callback, item)
			if err != nil {
				return Value{}, err
			}
			if !ok.IsTruthy() {
				return BoxBool(false), nil
			}
		}
		return BoxBool(true), nil
	}),
	newVariadicMethod("reduce", 1, func(c Call) (Value, error) {
		if c.Len() > 3 {
			return Value{}, Errorf(KindType, "reduce takes 1 or 2 arguments, got %d", c.Len()-1)
		}
		callback, err := c.Callable(1)
		if err != nil {
			return Value{}, err
		}
		data := c.This().mustArray().Data
		var acc Value
		switch {
		case c.Len() == 3:
			acc = c.Arg(2)
		case len(data) == 0:
			return Value{}, Errorf(KindValue, "reduce: an empty array needs a starting value, as in reduce(f, 0)")
		default:
			acc, data = data[0], data[1:]
		}
		for _, item := range data {
			if acc, err = c.Call(callback, acc, item); err != nil {
				return Value{}, err
			}
		}
		return acc, nil
	}),
	newVariadicMethod("sort", 0, func(c Call) (Value, error) {
		if c.Len() > 2 {
			return Value{}, Errorf(KindType, "sort takes 0 or 1 arguments, got %d", c.Len()-1)
		}
		sorted := slices.Clone(c.This().mustArray().Data)
		var err error
		if c.Len() == 1 {
			err = sortNatural(sorted)
		} else {
			err = sortBy(c, sorted)
		}
		if err != nil {
			return Value{}, err
		}
		return BoxArray(NewArray(sorted...)), nil
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
				return Value{}, Errorf(KindType, "join: element %d is %s, not a string", i, item.TypeOf())
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
	return BoxBool(c.This().mustObject().Has(key)), nil
})

var objectRemove = newMethod("remove", 1, func(c Call) (Value, error) {
	key, ok := c.Arg(1).AsString()
	if !ok {
		return Value{}, nil
	}
	obj := c.This().mustObject()
	val, exists := obj.Get(key)
	if exists {
		obj.Delete(key)
		return val, nil
	}
	return Value{}, nil
})

var typeObject = newBuiltinType("object", NewFunc("object", 0, func(c Call) (Value, error) {
	return BoxObject(NewObject(0)), nil
}), methodTable(
	newMethod("keys", 0, func(c Call) (Value, error) {
		keys := c.This().mustObject().keys
		arr := make([]Value, len(keys))
		for i, k := range keys {
			arr[i] = BoxString(k)
		}
		return BoxArray(NewArray(arr...)), nil
	}),
	newMethod("values", 0, func(c Call) (Value, error) {
		return BoxArray(NewArray(slices.Clone(c.This().mustObject().values)...)), nil
	}),
	newMethod("len", 0, func(c Call) (Value, error) {
		return BoxNumber(float64(c.This().mustObject().Len())), nil
	}),
	objectHas,
	alias("contains", objectHas),
	objectRemove,
	alias("delete", objectRemove),
	newMethod("clear", 0, func(c Call) (Value, error) {
		c.This().mustObject().Clear()
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
		return Value{}, Errorf(KindType, "buffer.push: argument must be a byte (0..255), string or buffer, got %s", arg.TypeOf())
	}
	if b < 0 || b > 255 {
		return Value{}, Errorf(KindValue, "buffer.push: byte value %d out of range 0..255", b)
	}
	*bp = append(*bp, byte(b))
	return c.This(), nil
})

var typeBuffer = newBuiltinType("buffer", NewVariadic("buffer", 0, func(c Call) (Value, error) {
	if c.Len() == 0 {
		return BoxBuffer([]byte{}), nil
	}
	if c.Len() > 1 {
		return Value{}, Errorf(KindType, "buffer takes at most 1 argument, got %d", c.Len())
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
				return Value{}, Errorf(KindValue, "buffer: array element %d must be an integer in 0..255", i)
			}
			out[i] = byte(int(f))
		}
		return BoxBuffer(out), nil
	}
	n, err := c.Int(0)
	if err != nil {
		return Value{}, Errorf(KindType, "buffer: argument must be a string, array, buffer or size, got %s", arg.TypeOf())
	}
	if n < 0 {
		return Value{}, Errorf(KindValue, "buffer: size must not be negative, got %d", n)
	}
	if n > maxStringAllocBytes {
		return Value{}, Errorf(KindValue, "buffer: size %d exceeds the maximum buffer size (%d bytes)", n, maxStringAllocBytes)
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
			return boxError(exc.clone(false)), nil // a value of its own, which catch can catch when thrown
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
