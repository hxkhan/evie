package core

import (
	"math"
	"strconv"
	"strings"
	"unsafe"
)

/*
NOTE: Theres 2 parts to a box.Value
	1. The scalar can store int64, float64, bool
	2. The pointer can store any reference value like strings, functions, arrays, maps or custom types provided by Go packages

When it's a scalar, the pointer tells us the type of the scalar.
When it's a reference, the scalar tells us the type of the reference.
So how do we what it is? We follow some basic rules.
This might sound confusing but I've tried about 4 different designs and this one came out the best.
Compared to using Golang's any type, this is in a different league altogether because scalars here are not heap allocated.

RULES:
	1. null:    it's enough for just the pointer to be nil
	2. bool:    the pointer has to be equal to boolType; the scalar is 0 for false else true
	4. int64:   the pointer has to be equal to i64Type; the scalar then stores the value
	4. float64: the pointer has to be equal to f64Type; the scalar then stores the value
	5. string:  the pointer has to be none of (boolType, i64Type, f64Type); the scalar has to be 0
	6. userFn:  the pointer has to be none of (boolType, i64Type, f64Type); the scalar has to be 1
	7. array:   the pointer has to be none of (boolType, i64Type, f64Type); the scalar has to be 2
	8. custom:  the pointer has to be none of (boolType, i64Type, f64Type); the scalar has to be 3

Another alternative to these two is using this exact same Value struct with different rules.
The scalar would use nan-tagging and would either be a valid float64 or a NaN and contain meta data that
would suggest if it's null, a bool, an int32 (if needed) or a reference value,
in the last case, we would use the pointer part of the struct and cast it to the appropriate type.
Although arguably simpler in design, we lose 64 bit integers so idk.
*/

const (
	scalarString = iota
	scalarUserFn
	scalarArray
	scalarCustom
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

var boolType unsafe.Pointer
var i64Type unsafe.Pointer
var f64Type unsafe.Pointer

func init() {
	boolType = unsafe.Pointer(new(byte))
	i64Type = unsafe.Pointer(new(byte))
	f64Type = unsafe.Pointer(new(byte))
}

// BoxFloat64 boxes a float64
func BoxFloat64(f float64) Value {
	return Value{scalar: math.Float64bits(f), pointer: f64Type}
}

// BoxInt64 boxes an int64
func BoxInt64(i int64) Value {
	return Value{scalar: uint64(i), pointer: i64Type}
}

// BoxBool boxes a boolean into
func BoxBool(b bool) Value {
	if b {
		return Value{scalar: 1, pointer: boolType}
	}
	return Value{scalar: 0, pointer: boolType}
}

// BoxString boxes a Golang string
func BoxString(str string) Value {
	return Value{scalar: scalarString, pointer: unsafe.Pointer(&str)}
}

// BoxUserFn boxes a user fn pointer
func BoxUserFn(ptr UserFn) Value {
	return Value{scalar: scalarUserFn, pointer: unsafe.Pointer(&ptr)}
}

// BoxArray boxes an evie array
func BoxArray(array []Value) Value {
	return Value{scalar: scalarArray, pointer: unsafe.Pointer(&array)}
}

// BoxCustom boxes a value of a custom type
func BoxCustom(cv CustomValue) Value {
	return Value{scalar: scalarCustom, pointer: unsafe.Pointer(&cv)}
}

func (v Value) IsNull() bool {
	return v.pointer == nil
}

func (v Value) AsFloat64() (float64, bool) {
	if v.pointer == f64Type {
		return math.Float64frombits(v.scalar), true
	}
	return 0, false
}

func (v Value) AsInt64() (int64, bool) {
	if v.pointer == i64Type {
		return int64(v.scalar), true
	}
	return 0, false
}

func (v Value) AsString() (string, bool) {
	switch v.pointer {
	case nil, i64Type, f64Type, boolType:
		return "", false
	}

	if v.scalar == scalarString {
		return *(*string)(v.pointer), true
	}

	return "", false
}

func (v Value) AsUserFn() (UserFn, bool) {
	switch v.pointer {
	case nil, i64Type, f64Type, boolType:
		return UserFn{}, false
	}

	if v.scalar == scalarUserFn {
		return *(*UserFn)(v.pointer), true
	}

	return UserFn{}, false
}

func (v Value) AsArray() ([]Value, bool) {
	switch v.pointer {
	case nil, i64Type, f64Type, boolType:
		return nil, false
	}

	if v.scalar == scalarArray {
		return *(*[]Value)(v.pointer), true
	}

	return nil, false
}

func (v Value) AsCustom() (CustomValue, bool) {
	switch v.pointer {
	case nil, i64Type, f64Type, boolType:
		return nil, false
	}

	if v.scalar == scalarCustom {
		return *(*CustomValue)(v.pointer), true
	}

	return nil, false
}

func (v Value) AsBool() (bool, bool) {
	return v.scalar != 0, v.pointer == boolType
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
	case scalarString: // string
		return *(*string)(v.pointer) != ""
	case scalarUserFn: // userFn
		// In both JavaScript and Python, functions are inherently truthy
		return true
	case scalarArray: // array
		array := *(*[]Value)(v.pointer)
		return len(array) != 0
	case scalarCustom:
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
	case scalarString: // string
		return *(*string)(a.pointer) == *(*string)(b.pointer)
	case scalarUserFn: // userFn
		return a.pointer == b.pointer
	case scalarArray: // array
		return a.pointer == b.pointer
	case scalarCustom:
		cvL := (*(*CustomValue)(a.pointer))
		cvR := (*(*CustomValue)(b.pointer))
		return cvL.Equals(cvR)
	}

	return false
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
	case scalarString: // string
		return *(*string)(v.pointer)
	case scalarUserFn: // userFn
		return "<fn>"
	case scalarArray: // array
		array := *(*[]Value)(v.pointer)

		builder := strings.Builder{}
		builder.WriteByte('[')

		for i, v := range array {
			if str, ok := v.AsString(); ok {
				builder.WriteByte('\'')
				builder.WriteString(str)
				builder.WriteByte('\'')
			} else {
				builder.WriteString(v.String())
			}

			if i != len(array)-1 {
				builder.WriteString(", ")
			}
		}

		builder.WriteByte(']')
		return builder.String()
	case scalarCustom: // custom
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
	case scalarString: // string
		return "string"
	case scalarUserFn: // userFn
		return "<fn>"
	case scalarArray: // array
		return "array"
	case scalarCustom: // custom
		cv := (*(*CustomValue)(v.pointer))
		return cv.TypeOf()
	}

	return "<unknown>"
}
