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

When it's a scalar, the pointer tells us the type of the scalar.
When it's a reference, the scalar tells us the type of the reference.
So how do we what it is? We follow some basic rules.
This might sound confusing but I've tried about 4 different designs and this one came out the best.
Compared to using Golang's any type, this is in a different league altogether because scalars here are not heap allocated.

RULES:
	1.  null:    it's enough for just the pointer to be nil
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

// Value stores the boxed value
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

func (v Value) IsNull() bool {
	return v.pointer == nil
}

func (v Value) AsInt64() (i int64, ok bool) {
	return int64(v.scalar), v.pointer == i64Type
}

func (v Value) AsFloat64() (f float64, ok bool) {
	return math.Float64frombits(v.scalar), v.pointer == f64Type
}

func (v Value) AsBool() (b bool, ok bool) {
	return v.scalar != 0, v.pointer == boolType
}

func (v Value) AsString() (s string, ok bool) {
	if isKnown(v.pointer) {
		return "", false
	}

	if v.scalar == stringType {
		return *(*string)(v.pointer), true
	}

	return "", false
}

func (v Value) AsUserFn() (fn UserFn, ok bool) {
	if isKnown(v.pointer) {
		return UserFn{}, false
	}

	if v.scalar == userFnType {
		return *(*UserFn)(v.pointer), true
	}

	return UserFn{}, false
}

func (v Value) AsNativeFn() (iface any, ok bool) {
	if isKnown(v.pointer) {
		return nil, false
	}

	if v.scalar == funcType {
		return *(*any)(v.pointer), true
	}

	return nil, false
}

func (v Value) AsArray() (array []Value, ok bool) {
	if isKnown(v.pointer) {
		return nil, false
	}

	if v.scalar == arrayType {
		return *(*[]Value)(v.pointer), true
	}

	return nil, false
}

func (v Value) AsTask() (task <-chan TaskResult, ok bool) {
	if isKnown(v.pointer) {
		return nil, false
	}

	if v.scalar == taskType {
		return *(*chan TaskResult)(v.pointer), true
	}

	return nil, false
}

func (v Value) AsBuffer() (buffer []byte, ok bool) {
	if isKnown(v.pointer) {
		return nil, false
	}

	if v.scalar == bufferType {
		return *(*[]byte)(v.pointer), true
	}

	return nil, false
}

func (v Value) AsCustom() (cv CustomValue, ok bool) {
	if isKnown(v.pointer) {
		return nil, false
	}

	if v.scalar == customType {
		return *(*CustomValue)(v.pointer), true
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

func (v Value) IsTruthy() bool {
	switch v.pointer {
	case nil:
		return false
	case boolType:
		return v.scalar != 0
	case i64Type:
		return int64(v.scalar) != 0
	case f64Type:
		return math.Float64frombits(v.scalar) != 0
	}

	switch v.scalar {
	case stringType:
		return *(*string)(v.pointer) != ""
	case userFnType:
		// In both JavaScript and Python, functions are inherently truthy
		return true
	case arrayType:
		array := *(*[]Value)(v.pointer)
		return len(array) != 0
	case taskType:
		task := *(*chan TaskResult)(v.pointer)
		return len(task) != 0
	case bufferType:
		array := *(*[]Value)(v.pointer)
		return len(array) != 0
	case customType:
		cv := *(*CustomValue)(v.pointer)
		return cv.IsTruthy()
	}

	return false
}

func (a Value) Equals(b Value) bool {
	switch a.pointer {
	case nil:
		return b.pointer == nil
	case boolType:
		return a.scalar == b.scalar
	case i64Type:
		return int64(a.scalar) == int64(b.scalar)
	case f64Type:
		return math.Float64frombits(a.scalar) == math.Float64frombits(b.scalar)
	}

	// guarantees that their types are the same beyond this point
	if a.scalar != b.scalar {
		return false
	}

	switch a.scalar {
	case stringType:
		return *(*string)(a.pointer) == *(*string)(b.pointer)
	case customType:
		lhs := (*(*CustomValue)(a.pointer))
		rhs := (*(*CustomValue)(b.pointer))
		return lhs.Equals(rhs)
	}

	// default comparison
	return a.pointer == b.pointer
}

func (v Value) String() string {
	switch v.pointer {
	case nil:
		return "null"
	case boolType:
		if v.scalar == 0 {
			return "false"
		}
		return "true"
	case i64Type:
		return strconv.FormatInt(int64(v.scalar), 10)
	case f64Type:
		return strconv.FormatFloat(math.Float64frombits(v.scalar), 'f', -1, 64)
	}

	switch v.scalar {
	case stringType:
		return *(*string)(v.pointer)
	case userFnType:
		return "<fn>"
	case arrayType:
		array := *(*[]Value)(v.pointer)

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
		array := *(*[]byte)(v.pointer)
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
		cv := (*(*CustomValue)(v.pointer))
		return cv.String()
	}

	return "<unknown>"
}

func (v Value) TypeOf() string {
	switch v.pointer {
	case nil:
		return "null"
	case boolType:
		return "bool"
	case i64Type:
		return "int"
	case f64Type:
		return "float"
	}

	switch v.scalar {
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
		cv := (*(*CustomValue)(v.pointer))
		return cv.TypeOf()
	}

	return "<unknown>"
}
