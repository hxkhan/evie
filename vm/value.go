package vm

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
	1.  nil:     the pointer has to be equal to nil; the scalar is irrelevant
	2.  bool:    the pointer has to be equal to boolType; the scalar is 0 for false else true
	3.  float64: the pointer has to be equal to f64Type; the scalar then stores the value
	4.  string:  the pointer has to be none of (f64Type, boolType); the scalar has to be stringType
	5.  userFn:  the pointer has to be none of (f64Type, boolType); the scalar has to be userFnType
	6.  func:    the pointer has to be none of (f64Type, boolType); the scalar has to be funcType
	7.  array:   the pointer has to be none of (f64Type, boolType); the scalar has to be arrayType
	8.  task:    the pointer has to be none of (f64Type, boolType); the scalar has to be taskType
	9.  buffer:  the pointer has to be none of (f64Type, boolType); the scalar has to be bufferType
	10. custom:  the pointer has to be none of (f64Type, boolType); the scalar has to be customType

Another alternative to these two is using this exact same Value struct with different rules.
The scalar would use nan-tagging and would either be a valid float64 or a NaN and contain meta data that
would suggest if it is nil, a bool, an int32 (if needed) or a reference value,
in the last case, we would use the pointer part of the struct and cast it to the appropriate type.
Although arguably simpler in design, we lose 64 bit integers so idk.
*/

// Value represents a boxed value
type Value struct {
	scalar  uint64
	pointer unsafe.Pointer
}

const (
	stringType = iota
	userFnType
	goFuncType
	arrayType
	taskType
	packageType
	bufferType
	customType
)

// scalar types
var f64Type = unsafe.Pointer(new(byte))
var boolType = unsafe.Pointer(new(byte))

// CustomValue is an interface for evie hosts to add their own custom values to the language
type CustomValue interface {
	String() string
	TypeOf() string
	IsTruthy() bool
	Equals(b CustomValue) bool
}

// GoFunc is a compile time safety interface so uncallable functions don't get into the system
type GoFunc interface {
	func() (Value, *Exception) |
		func(Value) (Value, *Exception) |
		func(Value, Value) (Value, *Exception) |
		func(Value, Value, Value) (Value, *Exception) |
		func(Value, Value, Value, Value) (Value, *Exception) |
		func(Value, Value, Value, Value, Value) (Value, *Exception) |
		func(Value, Value, Value, Value, Value, Value) (Value, *Exception)
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

// BoxGoFunc boxes a golang function
func BoxGoFunc[T GoFunc](fn T) Value {
	iface := any(fn)
	return Value{scalar: goFuncType, pointer: unsafe.Pointer(&iface)}
}

// BoxArray boxes an evie array
func BoxArray(array []Value) Value {
	return Value{scalar: arrayType, pointer: unsafe.Pointer(&array)}
}

// BoxTask boxes an evie task
func BoxTask(task chan evaluation) Value {
	return Value{scalar: taskType, pointer: unsafe.Pointer(&task)}
}

// BoxPackage boxes an evie package
/* func BoxPackage(pkg Package) Value {
	return Value{scalar: packageType, pointer: unsafe.Pointer(pkg.(*packageInstance))}
} */

// Box boxes an evie package
func (pkg *packageInstance) Box() Value {
	return Value{scalar: packageType, pointer: unsafe.Pointer(pkg)}
}

// BoxBuffer boxes a Golang byte slice
func BoxBuffer(bytes []byte) Value {
	return Value{scalar: bufferType, pointer: unsafe.Pointer(&bytes)}
}

// BoxCustom boxes a value of a custom type
func BoxCustom(cv CustomValue) Value {
	return Value{scalar: customType, pointer: unsafe.Pointer(&cv)}
}

func (x Value) IsNil() bool {
	return x.pointer == nil
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
	if x.scalar != userFnType || isKnown(x.pointer) {
		return nil, false
	}
	return (*UserFn)(x.pointer), true
}

func (x Value) AsGoFunc() (iface any, ok bool) {
	if x.scalar != goFuncType || isKnown(x.pointer) {
		return nil, false
	}
	return *(*any)(x.pointer), true
}

func (x Value) AsArray() (array []Value, ok bool) {
	if x.scalar != arrayType || isKnown(x.pointer) {
		return nil, false
	}
	return *(*[]Value)(x.pointer), true
}

func (x Value) AsTask() (task <-chan evaluation, ok bool) {
	if x.scalar != taskType || isKnown(x.pointer) {
		return nil, false
	}
	return *(*chan evaluation)(x.pointer), true
}

func (x Value) asPackage() (pkg *packageInstance, ok bool) {
	if x.scalar != packageType || isKnown(x.pointer) {
		return nil, false
	}
	return (*packageInstance)(x.pointer), true
}

func (x Value) AsPackage() (pkg Package, ok bool) {
	if x.scalar != packageType || isKnown(x.pointer) {
		return nil, false
	}
	return (*packageInstance)(x.pointer), true
}

func (x Value) AsBuffer() (buffer []byte, ok bool) {
	if x.scalar != bufferType || isKnown(x.pointer) {
		return nil, false
	}
	return *(*[]byte)(x.pointer), true
}

func (x Value) AsCustom() (cv CustomValue, ok bool) {
	if x.scalar != customType || isKnown(x.pointer) {
		return nil, false
	}
	return *(*CustomValue)(x.pointer), true
}

// Allocate will copy the current value to the heap and return a pointer to it
func (x Value) Allocate() *Value {
	return &x
}

func isKnown(p unsafe.Pointer) bool {
	switch p {
	case nil, f64Type, boolType:
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
		task := *(*chan evaluation)(x.pointer)
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
		return "nil"
	case boolType:
		if x.scalar == 0 {
			return "false"
		}
		return "true"
	case f64Type:
		return strconv.FormatFloat(math.Float64frombits(x.scalar), 'f', -1, 64)
	}

	switch x.scalar {
	case stringType:
		return *(*string)(x.pointer)
	case userFnType:
		return (*UserFn)(x.pointer).String()
	case goFuncType:
		return "<function>"
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
	case packageType:
		return "<package>"
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
		return "nil"
	case boolType:
		return "bool"
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
