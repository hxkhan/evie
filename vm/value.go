package vm

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	"github.com/hxkhan/evie/ast"
	"github.com/hxkhan/evie/vm/fields"
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
	10. error:   the pointer has to be none of (f64Type, boolType); the scalar has to be errorType
	11. custom:  the pointer has to be none of (f64Type, boolType); the scalar has to be customType

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
	kindString = iota
	kindUserFn
	kindGoFunc
	kindMethod
	kindArray
	kindTask
	kindPackage
	kindBuffer
	kindObject
	kindBuiltinType
	kindStruct
	kindStructInstance
	kindCustom
)

// scalar types
var f64Type = unsafe.Pointer(new(byte))
var boolType = unsafe.Pointer(new(byte))

// type ids
var strTypeID = unsafe.Pointer(new(byte))
var arrayTypeID = unsafe.Pointer(new(byte))

// CustomValue is an interface for evie hosts to add their own custom values to the language
type CustomValue interface {
	String() string
	TypeOf() string
	IsTruthy() bool
	Equals(b CustomValue) bool
}

// SafeGoFunc is a compile time safety interface so uncallable functions don't get into the system
type SafeGoFunc interface {
	func() (Value, Exception) |
		func(Value) (Value, Exception) |
		func(Value, Value) (Value, Exception) |
		func(Value, Value, Value) (Value, Exception) |
		func(Value, Value, Value, Value) (Value, Exception) |
		func(Value, Value, Value, Value, Value) (Value, Exception) |
		func(Value, Value, Value, Value, Value, Value) (Value, Exception)
}

// BoxNumber boxes a float64
func BoxNumber(f float64) Value {
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
	return Value{scalar: kindString, pointer: unsafe.Pointer(&str)}
}

// BoxUserFn boxes an evie function
func BoxUserFn(fn UserFn) Value {
	return Value{scalar: kindUserFn, pointer: unsafe.Pointer(&fn)}
}

// BoxBuiltinType boxes a Go function that is a type constructor
func BoxBuiltinType[T SafeGoFunc](name string, constructor T) Value {
	ptr := unsafe.Pointer(&BuiltinType{
		Name: name,
		Constructor: GoFunc{
			nargs: reflect.TypeOf(constructor).NumIn(),
			ptr:   unsafe.Pointer(&constructor),
			mode:  ast.UndefinedMode,
		},
	})
	return Value{scalar: kindBuiltinType, pointer: ptr}
}

// BoxGoFunc boxes a sync-agnostic Go function
func BoxGoFunc[T SafeGoFunc](fn T) Value {
	ptr := unsafe.Pointer(&GoFunc{
		nargs: reflect.TypeOf(fn).NumIn(),
		ptr:   unsafe.Pointer(&fn),
		mode:  ast.UndefinedMode,
	})
	return Value{scalar: kindGoFunc, pointer: ptr}
}

// BoxGoFunc boxes a synced Go function always assuming the safety of the GIL
func BoxGoFuncSynced[T SafeGoFunc](fn T) Value {
	ptr := unsafe.Pointer(&GoFunc{
		nargs: reflect.TypeOf(fn).NumIn(),
		ptr:   unsafe.Pointer(&fn),
		mode:  ast.SyncedMode,
	})
	return Value{scalar: kindGoFunc, pointer: ptr}
}

// BoxArray boxes an evie array
func BoxArray(arr *Array) Value {
	return Value{scalar: kindArray, pointer: unsafe.Pointer(arr)}
}

// BoxObject boxes an evie object
func BoxObject(obj map[fields.ID]Value) Value {
	return Value{scalar: kindObject, pointer: unsafe.Pointer(&obj)}
}

// BoxUserStruct boxes an evie struct
func BoxUserStruct(obj *UserStruct) Value {
	return Value{scalar: kindStruct, pointer: unsafe.Pointer(obj)}
}

// BoxUserStructInstance boxes an evie struct instance
func BoxUserStructInstance(obj *UserStructInstance) Value {
	return Value{scalar: kindStructInstance, pointer: unsafe.Pointer(obj)}
}

// BoxTask boxes an evie task
func BoxTask(task chan evaluation) Value {
	return Value{scalar: kindTask, pointer: unsafe.Pointer(&task)}
}

// BoxPackage boxes an evie package
/* func BoxPackage(pkg Package) Value {
	return Value{scalar: packageType, pointer: unsafe.Pointer(pkg.(*packageInstance))}
} */

// Box boxes an evie package
func (pkg *packageInstance) Box() Value {
	return Value{scalar: kindPackage, pointer: unsafe.Pointer(pkg)}
}

func boxMethod(m Method) Value {
	return Value{scalar: kindMethod, pointer: unsafe.Pointer(&m)}
}

// BoxBuffer boxes a Golang byte slice
func BoxBuffer(bytes []byte) Value {
	return Value{scalar: kindBuffer, pointer: unsafe.Pointer(&bytes)}
}

// BoxCustom boxes a value of a custom type
func BoxCustom(cv CustomValue) Value {
	return Value{scalar: kindCustom, pointer: unsafe.Pointer(&cv)}
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

	if x.scalar == kindString {
		return *(*string)(x.pointer), true
	}

	return "", false
}

func (x Value) AsUserFn() (fn *UserFn, ok bool) {
	if x.scalar != kindUserFn || isKnown(x.pointer) {
		return nil, false
	}
	return (*UserFn)(x.pointer), true
}

func (x Value) AsGoFunc() (fn *GoFunc, ok bool) {
	if x.scalar != kindGoFunc || isKnown(x.pointer) {
		return nil, false
	}
	return (*GoFunc)(x.pointer), true
}

func (x Value) AsArray() (arr *Array, ok bool) {
	if x.scalar != kindArray || isKnown(x.pointer) {
		return nil, false
	}
	return (*Array)(x.pointer), true
}

func (x Value) AsObject() (obj map[fields.ID]Value, ok bool) {
	if x.scalar != kindObject || isKnown(x.pointer) {
		return nil, false
	}
	return *(*map[fields.ID]Value)(x.pointer), true
}

func (x Value) AsUserStruct() (obj *UserStruct, ok bool) {
	if x.scalar != kindStruct || isKnown(x.pointer) {
		return nil, false
	}
	return (*UserStruct)(x.pointer), true
}

func (x Value) AsUserStructInstance() (obj *UserStructInstance, ok bool) {
	if x.scalar != kindStructInstance || isKnown(x.pointer) {
		return nil, false
	}
	return (*UserStructInstance)(x.pointer), true
}

func (x Value) AsBuiltinType() (obj *BuiltinType, ok bool) {
	if x.scalar != kindBuiltinType || isKnown(x.pointer) {
		return nil, false
	}
	return (*BuiltinType)(x.pointer), true
}

func (x Value) AsTask() (task <-chan evaluation, ok bool) {
	if x.scalar != kindTask || isKnown(x.pointer) {
		return nil, false
	}
	return *(*chan evaluation)(x.pointer), true
}

func (x Value) asPackage() (pkg *packageInstance, ok bool) {
	if x.scalar != kindPackage || isKnown(x.pointer) {
		return nil, false
	}
	return (*packageInstance)(x.pointer), true
}

func (x Value) AsPackage() (pkg Package, ok bool) {
	if x.scalar != kindPackage || isKnown(x.pointer) {
		return nil, false
	}
	return (*packageInstance)(x.pointer), true
}

func (x Value) asMethod() (m *Method, ok bool) {
	if x.scalar != kindMethod || isKnown(x.pointer) {
		return nil, false
	}
	return (*Method)(x.pointer), true
}

func (x Value) AsBuffer() (buffer []byte, ok bool) {
	if x.scalar != kindBuffer || isKnown(x.pointer) {
		return nil, false
	}
	return *(*[]byte)(x.pointer), true
}

func (x Value) AsCustom() (cv CustomValue, ok bool) {
	if x.scalar != kindCustom || isKnown(x.pointer) {
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
	case kindString:
		return *(*string)(x.pointer) != ""
	case kindUserFn:
		// In both JavaScript and Python, functions are inherently truthy
		return true
	case kindArray:
		array := (*Array)(x.pointer)
		return len(array.Data) != 0
	case kindObject:
		obj := *(*map[fields.ID]Value)(x.pointer)
		return len(obj) != 0
	case kindTask:
		task := *(*chan evaluation)(x.pointer)
		return len(task) != 0
	case kindBuffer:
		buffer := *(*[]byte)(x.pointer)
		return len(buffer) != 0
	case kindCustom:
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
	case kindString:
		return *(*string)(x.pointer) == *(*string)(y.pointer)
	case kindCustom:
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
	case kindString:
		return *(*string)(x.pointer)
	case kindUserFn:
		return (*UserFn)(x.pointer).String()
	case kindStructInstance:
		return (*UserStructInstance)(x.pointer).String()
	case kindGoFunc:
		return "<function>"
	case kindArray:
		return (*Array)(x.pointer).String()

	case kindObject:
		obj := *(*map[fields.ID]Value)(x.pointer)

		builder := strings.Builder{}
		builder.WriteByte('{')

		iter := 0
		for i, v := range obj {
			builder.WriteString(fields.Lookup(i))
			builder.WriteByte(':')
			builder.WriteByte(' ')

			if str, ok := v.AsString(); ok {
				builder.WriteByte('"')
				builder.WriteString(str)
				builder.WriteByte('"')
			} else {
				builder.WriteString(v.String())
			}

			if iter != len(obj)-1 {
				builder.WriteString(", ")
			}

			iter += 1
		}

		builder.WriteByte('}')
		return builder.String()

	case kindStruct:
		obj := (*UserStruct)(x.pointer)
		return fmt.Sprintf("<type %v>", obj.Name)
	case kindTask:
		return "<task>"
	case kindPackage:
		return "<package>"
	case kindMethod:
		return "<method>"
	case kindBuiltinType:
		obj := (*BuiltinType)(x.pointer)
		return fmt.Sprintf("<type %v>", obj.Name)
	case kindBuffer:
		return fmt.Sprintf("<buffer %v>", x.pointer)
	case kindCustom:
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
		return "number"
	}

	switch x.scalar {
	case kindString:
		return "string"
	case kindUserFn:
		return "function"
	case kindGoFunc:
		return "function"
	case kindArray:
		return "array"
	case kindObject:
		return "object"
	case kindTask:
		return "task"
	case kindPackage:
		return "package"
	case kindMethod:
		return "method"
	case kindBuffer:
		return "buffer"
	case kindStruct:
		return "error"
	case kindCustom:
		cv := (*(*CustomValue)(x.pointer))
		return cv.TypeOf()
	}

	return "<unknown>"
}

func (x Value) TypeID() unsafe.Pointer {
	if isKnown(x.pointer) {
		return x.pointer
	}

	switch x.scalar {
	case kindString:
		return strTypeID
	case kindArray:
		return arrayTypeID
	case kindPackage:
		return x.pointer
	}

	panic("TypeID() -> cant figure it out...")
}

func (x Value) getField(f fields.ID) (field Value, ok bool) {
	if isKnown(x.pointer) {
		return Value{}, false
	}

	switch x.scalar {
	case kindString:
		value, exists := stringMethods[f]
		if !exists {
			return Value{}, false
		}

		m := Method{this: x, fn: *value}
		return boxMethod(m), true

	case kindArray:
		value, exists := arrayMethods[f]
		if !exists {
			return Value{}, false
		}

		m := Method{this: x, fn: *value}
		return boxMethod(m), true

	case kindObject:
		obj := *(*map[fields.ID]Value)(x.pointer)

		value, exists := obj[f]
		if !exists {
			return Value{}, false
		}

		return value, true

	case kindStructInstance:
		obj := (*UserStructInstance)(x.pointer)

		value, exists := obj.Fields[f]
		if !exists {
			return Value{}, false
		}

		return value, true

	case kindPackage:
		pkg := (*packageInstance)(x.pointer)
		value, exists := pkg.globals[f]
		if !value.IsPublic {
			return Value{}, false
		}
		return *(value.Value), exists
	}

	return Value{}, false
}

func (x Value) dotAccess(f fields.ID) (field *Value) {
	if isKnown(x.pointer) {
		return nil
	}

	switch x.scalar {
	case kindString:
		return stringMethods[f]
	case kindArray:
		return arrayMethods[f]
	case kindPackage:
		pkg := (*packageInstance)(x.pointer)
		value := pkg.globals[f]
		if !value.IsPublic {
			return nil
		}
		return value.Value
	}

	panic("add more types?")
}
