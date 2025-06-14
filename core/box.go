package core

import (
	"math"
	"strconv"
	"strings"
	"unsafe"
)

/*
NOTE: Theres 2 parts to a Value
	1. The scalar can store int64, float64, bool
	2. The pointer can store any reference value like strings, functions, arrays, maps or custom types provided by Go packages

When we are storing a scalar, the pointer tells us the type of the scalar.
When we are storing a reference, the scalar tells us the type of the reference.
So how do we know what it is? We follow some basic rules.

RULES:
	1.  null:    the pointer has to be equal to nil; the scalar can (optionally) be 0
	2.  bool:    the pointer has to be equal to boolType; the scalar is 0 for false else true
	4.  int64:   the pointer has to be equal to i64Type; the scalar then stores the value
	4.  float64: the pointer has to be equal to f64Type; the scalar then stores the value
	5.  string:  the pointer has to be none of (i64Type, f64Type, boolType); the scalar has to be stringType
	6.  userFn:  the pointer has to be none of (i64Type, f64Type, boolType); the scalar has to be userFnType
	6.  func:    the pointer has to be none of (i64Type, f64Type, boolType); the scalar has to be funcType
	7.  array:   the pointer has to be none of (i64Type, f64Type, boolType); the scalar has to be arrayType
	8.  task:    the pointer has to be none of (i64Type, f64Type, boolType); the scalar has to be taskType
	9.  buffer:  the pointer has to be none of (i64Type, f64Type, boolType); the scalar has to be bufferType
	10. custom:  the pointer has to be none of (i64Type, f64Type, boolType); the scalar has to be customType

Another alternative to these two is using this exact same Value struct with different rules.
The scalar would use nan-tagging and would either be a valid float64 or a NaN and contain meta data that
would suggest if it's null, a bool, an int32 (if needed) or a reference value,
in the last case, we would use the pointer part of the struct and cast it to the appropriate type.
Although arguably simpler in design, we lose 64 bit integers so idk.
*/

const (
	stringType = iota
	userFnType
	funcType
	arrayType
	taskType
	bufferType
	customType
)

type CustomValue interface {
	String() string
	TypeOf() string
	IsTruthy() bool
	Equals(b CustomValue) bool
}

// Value represents a boxed value
type Value struct {
	scalar  uint64
	pointer unsafe.Pointer
}

// scalar types
var i64Type = unsafe.Pointer(new(byte))
var f64Type = unsafe.Pointer(new(byte))
var boolType = unsafe.Pointer(new(byte))

// BoxInt64 boxes an int64
func BoxInt64(i int64) Value {
	return Value{scalar: uint64(i), pointer: i64Type}
}

// BoxFloat64 boxes a float64
func BoxFloat64(f float64) Value {
	return Value{scalar: math.Float64bits(f), pointer: f64Type}
}

// BoxBool boxes a boolean into
func BoxBool(b bool) Value {
	if b {
		return Value{scalar: 1, pointer: boolType}
	}
	return Value{scalar: 0, pointer: boolType}
}

// BoxString boxes a string
func BoxString(str string) Value {
	return Value{scalar: stringType, pointer: unsafe.Pointer(&str)}
}

// BoxUserFn boxes an evie function
func BoxUserFn(fn UserFn) Value {
	return Value{scalar: userFnType, pointer: unsafe.Pointer(&fn)}
}

// BoxFunc boxes a golang function
func BoxFunc[T ValidFuncTypes](fn T) Value {
	iface := any(fn)
	return Value{scalar: funcType, pointer: unsafe.Pointer(&iface)}
}

// BoxArray boxes an evie array
func BoxArray(array []Value) Value {
	return Value{scalar: arrayType, pointer: unsafe.Pointer(&array)}
}

// BoxTask boxes an evie task
func BoxTask(task chan TaskResult) Value {
	return Value{scalar: taskType, pointer: unsafe.Pointer(&task)}
}

// BoxBuffer boxes a Golang byte slice
func BoxBuffer(bytes []byte) Value {
	return Value{scalar: bufferType, pointer: unsafe.Pointer(&bytes)}
}

// BoxCustom boxes a value of a custom type
func BoxCustom(cv CustomValue) Value {
	return Value{scalar: customType, pointer: unsafe.Pointer(&cv)}
}

func (x Value) IsNull() bool {
	return x.pointer == nil
}

func (x Value) AsInt64() (i int64, ok bool) {
	return int64(x.scalar), x.pointer == i64Type
}

func (x Value) AsFloat64() (f float64, ok bool) {
	return math.Float64frombits(x.scalar), x.pointer == f64Type
}

func (x Value) AsBool() (b bool, ok bool) {
	return x.scalar != 0, x.pointer == boolType
}

func (x Value) AsString() (s string, ok bool) {
	if isKnown(x.pointer) {
		return "", false
	}

	if x.scalar == stringType {
		return *(*string)(x.pointer), true
	}

	return "", false
}

func (x Value) AsUserFn() (fn *UserFn, ok bool) {
	if isKnown(x.pointer) {
		return nil, false
	}

	if x.scalar == userFnType {
		return (*UserFn)(x.pointer), true
	}

	return nil, false
}

func (x Value) AsNativeFn() (iface any, ok bool) {
	if isKnown(x.pointer) {
		return nil, false
	}

	if x.scalar == funcType {
		return *(*any)(x.pointer), true
	}

	return nil, false
}

func (x Value) AsArray() (array []Value, ok bool) {
	if isKnown(x.pointer) {
		return nil, false
	}

	if x.scalar == arrayType {
		return *(*[]Value)(x.pointer), true
	}

	return nil, false
}

func (x Value) AsTask() (task <-chan TaskResult, ok bool) {
	if isKnown(x.pointer) {
		return nil, false
	}

	if x.scalar == taskType {
		return *(*chan TaskResult)(x.pointer), true
	}

	return nil, false
}

func (x Value) AsBuffer() (buffer []byte, ok bool) {
	if isKnown(x.pointer) {
		return nil, false
	}

	if x.scalar == bufferType {
		return *(*[]byte)(x.pointer), true
	}

	return nil, false
}

func (x Value) AsCustom() (cv CustomValue, ok bool) {
	if isKnown(x.pointer) {
		return nil, false
	}

	if x.scalar == customType {
		return *(*CustomValue)(x.pointer), true
	}

	return nil, false
}

func isKnown(p unsafe.Pointer) bool {
	switch p {
	case nil, i64Type, f64Type, boolType:
		return true
	}
	return false
}

func (x Value) IsTruthy() bool {
	switch x.pointer {
	case nil:
		return false
	case boolType:
		return x.scalar != 0
	case i64Type:
		return int64(x.scalar) != 0
	case f64Type:
		return math.Float64frombits(x.scalar) != 0
	}

	switch x.scalar {
	case stringType:
		return *(*string)(x.pointer) != ""
	case userFnType:
		// In both JavaScript and Python, functions are inherently truthy
		return true
	case arrayType:
		array := *(*[]Value)(x.pointer)
		return len(array) != 0
	case taskType:
		task := *(*chan TaskResult)(x.pointer)
		return len(task) != 0
	case bufferType:
		array := *(*[]Value)(x.pointer)
		return len(array) != 0
	case customType:
		cv := *(*CustomValue)(x.pointer)
		return cv.IsTruthy()
	}

	return false
}

func (x Value) Equals(y Value) bool {
	switch x.pointer {
	case nil:
		return y.pointer == nil
	case boolType:
		return x.scalar == y.scalar
	case i64Type:
		return int64(x.scalar) == int64(y.scalar)
	case f64Type:
		return math.Float64frombits(x.scalar) == math.Float64frombits(y.scalar)
	}

	// guarantees that their types are the same beyond this point
	if x.scalar != y.scalar {
		return false
	}

	switch x.scalar {
	case stringType:
		return *(*string)(x.pointer) == *(*string)(y.pointer)
	case customType:
		lhs := (*(*CustomValue)(x.pointer))
		rhs := (*(*CustomValue)(y.pointer))
		return lhs.Equals(rhs)
	}

	// default comparison
	return x.pointer == y.pointer
}

func (x Value) String() string {
	switch x.pointer {
	case nil:
		return "null"
	case boolType:
		if x.scalar == 0 {
			return "false"
		}
		return "true"
	case i64Type:
		return strconv.FormatInt(int64(x.scalar), 10)
	case f64Type:
		return strconv.FormatFloat(math.Float64frombits(x.scalar), 'f', -1, 64)
	}

	switch x.scalar {
	case stringType:
		return *(*string)(x.pointer)
	case userFnType:
		return "<fn>"
	case arrayType:
		array := *(*[]Value)(x.pointer)

		builder := strings.Builder{}
		builder.WriteByte('[')

		for i, v := range array {
			if str, ok := v.AsString(); ok {
				builder.WriteByte('"')
				builder.WriteString(str)
				builder.WriteByte('"')
			} else {
				builder.WriteString(v.String())
			}

			if i != len(array)-1 {
				builder.WriteString(", ")
			}
		}

		builder.WriteByte(']')
		return builder.String()

	case taskType:
		return "<task>"
	case bufferType:
		array := *(*[]byte)(x.pointer)
		builder := strings.Builder{}
		builder.WriteByte('[')

		for i, v := range array {
			builder.WriteByte('\'')
			builder.WriteString(strconv.FormatInt(int64(v), 10))
			builder.WriteByte('\'')

			if i != len(array)-1 {
				builder.WriteString(", ")
			}
		}

		builder.WriteByte(']')
		return builder.String()
	case customType:
		cv := (*(*CustomValue)(x.pointer))
		return cv.String()
	}

	return "<unknown>"
}

func (x Value) TypeOf() string {
	switch x.pointer {
	case nil:
		return "null"
	case boolType:
		return "bool"
	case i64Type:
		return "int"
	case f64Type:
		return "float"
	}

	switch x.scalar {
	case stringType:
		return "string"
	case userFnType:
		return "function"
	case arrayType:
		return "array"
	case taskType:
		return "task"
	case bufferType:
		return "buffer"
	case customType:
		cv := (*(*CustomValue)(x.pointer))
		return cv.TypeOf()
	}

	return "<unknown>"
}
