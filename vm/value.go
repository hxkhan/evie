package vm

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unsafe"

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
	kindBoundMethod
	kindArray
	kindTask
	kindPackage
	kindBuffer
	kindObject
	kindBuiltinType
	kindStruct
	kindStructInstance
	kindError
	kindHost
)

// scalar types
var f64Type = unsafe.Pointer(new(byte))
var boolType = unsafe.Pointer(new(byte))

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

// boxUserFnPtr boxes an existing function without copying it
func boxUserFnPtr(fn *UserFn) Value {
	return Value{scalar: kindUserFn, pointer: unsafe.Pointer(fn)}
}

// BoxBuiltinType boxes a Go function that is a type constructor
func BoxBuiltinType(t *BuiltinType) Value {
	return Value{scalar: kindBuiltinType, pointer: unsafe.Pointer(t)}
}

// BoxGoFunc boxes a Go function
func BoxGoFunc(fn *GoFunc) Value {
	return Value{scalar: kindGoFunc, pointer: unsafe.Pointer(fn)}
}

// BoxArray boxes an evie array
func BoxArray(arr *Array) Value {
	return Value{scalar: kindArray, pointer: unsafe.Pointer(arr)}
}

// BoxObject boxes an evie object
func BoxObject(obj map[string]Value) Value {
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
func BoxTask(task *Task) Value {
	return Value{scalar: kindTask, pointer: unsafe.Pointer(task)}
}

// BoxError boxes an evie error
func BoxError(err *Error) Value {
	return Value{scalar: kindError, pointer: unsafe.Pointer(err)}
}

func boxMethod(m BoundMethod) Value {
	return Value{scalar: kindBoundMethod, pointer: unsafe.Pointer(&m)}
}

// BoxBuffer boxes a Golang byte slice
func BoxBuffer(bytes []byte) Value {
	return Value{scalar: kindBuffer, pointer: unsafe.Pointer(&bytes)}
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

func (x Value) AsObject() (obj map[string]Value, ok bool) {
	if x.scalar != kindObject || isKnown(x.pointer) {
		return nil, false
	}
	return *(*map[string]Value)(x.pointer), true
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

func (x Value) AsTask() (task *Task, ok bool) {
	if x.scalar != kindTask || isKnown(x.pointer) {
		return nil, false
	}
	return (*Task)(x.pointer), true
}

func (x Value) AsPackage() (pkg *Package, ok bool) {
	if x.scalar != kindPackage || isKnown(x.pointer) {
		return nil, false
	}
	return (*Package)(x.pointer), true
}

func (x Value) AsError() (err *Error, ok bool) {
	if x.scalar != kindError || isKnown(x.pointer) {
		return nil, false
	}
	return (*Error)(x.pointer), true
}

func (x Value) asMethod() (m *BoundMethod, ok bool) {
	if x.scalar != kindBoundMethod || isKnown(x.pointer) {
		return nil, false
	}
	return (*BoundMethod)(x.pointer), true
}

func (x Value) AsBuffer() (buffer []byte, ok bool) {
	if x.scalar != kindBuffer || isKnown(x.pointer) {
		return nil, false
	}
	return *(*[]byte)(x.pointer), true
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
		obj := *(*map[string]Value)(x.pointer)
		return len(obj) != 0
	case kindTask:
		return true
	case kindBuffer:
		buffer := *(*[]byte)(x.pointer)
		return len(buffer) != 0
	case kindHost:
		return true
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
	case kindHost:
		return x.host().equals(y.host())
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
		//return (*UserFn)(x.pointer).String()
		return "callable"
	case kindStructInstance:
		return (*UserStructInstance)(x.pointer).String()
	case kindGoFunc:
		fn := (*GoFunc)(x.pointer)
		if fn.IsMethod {
			return "<callable>"
		}
		return "<callable>"

	case kindArray:
		return (*Array)(x.pointer).String()

	case kindObject:
		obj := *(*map[string]Value)(x.pointer)

		builder := strings.Builder{}
		builder.WriteByte('{')

		for i, name := range sortedKeys(obj) {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(name)
			builder.WriteString(": ")

			v := obj[name]
			if str, ok := v.AsString(); ok {
				builder.WriteByte('"')
				builder.WriteString(str)
				builder.WriteByte('"')
			} else {
				builder.WriteString(v.String())
			}
		}

		builder.WriteByte('}')
		return builder.String()

	case kindStruct:
		obj := (*UserStruct)(x.pointer)
		return fmt.Sprintf("<type %v>", obj.Name)
	case kindTask:
		t := (*Task)(x.pointer)
		if t.IsDone() {
			if t.err != nil {
				return "<task failed>"
			}
			return "<task completed>"
		}
		return "<task pending>"
	case kindPackage:
		return "<package>"
	case kindBoundMethod:
		//m := (*BoundMethod)(x.pointer)
		return "<callable>"
	case kindBuiltinType:
		obj := (*BuiltinType)(x.pointer)
		return fmt.Sprintf("<type %v>", obj.Name)
	case kindBuffer:
		return fmt.Sprintf("<buffer %v>", x.pointer)
	case kindError:
		obj := (*Error)(x.pointer)
		return obj.Error()
	case kindHost:
		return x.host().String()
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
	case kindBoundMethod:
		return "method"
	case kindBuffer:
		return "buffer"
	case kindStruct:
		return "struct"
	case kindStructInstance:
		return "struct"
	case kindBuiltinType:
		return "type"
	case kindError:
		return "error"
	case kindHost:
		return x.host().typ.Name()
	}

	return "<unknown>"
}

// fieldKey names a member that appears as a literal name in the source. Objects
// are keyed by the name itself; structs, packages and builtin types by the
// interned id, which the compiler resolves once.
type fieldKey struct {
	id   fields.ID
	name string
}

func fieldKeyOf(name string) fieldKey {
	return fieldKey{id: fields.Get(name), name: name}
}

var (
	messageID = fields.Get("message")
	lineID    = fields.Get("line")
	columnID  = fields.Get("column")
	fileID    = fields.Get("file")
	traceID   = fields.Get("trace")
)

// sortedKeys returns the keys of a string-keyed map in order
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (x Value) getField(k fieldKey) (field Value, ok bool) {
	if isKnown(x.pointer) {
		return Value{}, false
	}

	switch x.scalar {
	case kindString:
		return x.boundMethod(typeString, k.id)
	case kindArray:
		return x.boundMethod(typeArray, k.id)
	case kindBuffer:
		return x.boundMethod(typeBuffer, k.id)
	case kindTask:
		return x.boundMethod(typeTask, k.id)

	case kindObject:
		obj := *(*map[string]Value)(x.pointer)
		if value, exists := obj[k.name]; exists {
			return value, true
		}
		return x.boundMethod(typeObject, k.id)

	case kindBuiltinType:
		obj := (*BuiltinType)(x.pointer)
		value, exists := obj.Fields[k.id]
		return value, exists

	case kindStructInstance:
		obj := (*UserStructInstance)(x.pointer)
		if value, exists := obj.GetField(k.id); exists {
			return value, true
		}
		if method := obj.InstanceOf.lookupMethod(k.id); method != nil {
			return boxMethod(BoundMethod{this: x, fn: boxUserFnPtr(method)}), true
		}
		return Value{}, false

	case kindPackage:
		pkg := (*Package)(x.pointer)
		value, exists := pkg.globals[k.id]
		if !exists || !value.IsPublic {
			return Value{}, false
		}
		return *(value.Value), true

	case kindError:
		if v, ok := (*Error)(x.pointer).field(k.id); ok {
			return v, true
		}
		return x.boundMethod(typeError, k.id)

	case kindHost:
		return x.boundMethod(x.host().typ.typ, k.id)
	}

	return Value{}, false
}

// memberNames lists the members a value has, for suggestions
func (x Value) memberNames() []string {
	ids := func(table map[fields.ID]Value) []string {
		names := make([]string, 0, len(table))
		for id := range table {
			names = append(names, fields.Lookup(id))
		}
		return names
	}
	if isKnown(x.pointer) {
		return nil
	}
	switch x.scalar {
	case kindString:
		return ids(typeString.Fields)
	case kindArray:
		return ids(typeArray.Fields)
	case kindBuffer:
		return ids(typeBuffer.Fields)
	case kindTask:
		return ids(typeTask.Fields)
	case kindError:
		return append(ids(typeError.Fields), "message", "line", "column", "file", "trace")
	case kindObject:
		return append(sortedKeys(*(*map[string]Value)(x.pointer)), ids(typeObject.Fields)...)
	case kindBuiltinType:
		return ids((*BuiltinType)(x.pointer).Fields)
	case kindHost:
		return ids(x.host().typ.typ.Fields)
	case kindPackage:
		var names []string
		for id := range (*Package)(x.pointer).globals {
			names = append(names, fields.Lookup(id))
		}
		return names
	case kindStructInstance:
		usi := (*UserStructInstance)(x.pointer)
		var names []string
		for cur := usi.InstanceOf; cur != nil; cur = cur.Parent {
			for _, f := range cur.Fields {
				names = append(names, fields.Lookup(f))
			}
			for id := range cur.Methods {
				names = append(names, fields.Lookup(id))
			}
		}
		return append(names, sortedKeys(usi.Dynamic)...)
	}
	return nil
}

// boundMethod looks a method up on a builtin type and binds it to x
func (x Value) boundMethod(t *BuiltinType, f fields.ID) (Value, bool) {
	value, exists := t.Fields[f]
	if !exists {
		return Value{}, false
	}
	return boxMethod(BoundMethod{this: x, fn: value}), true
}

// dotAccess returns the member a call site like x.name(...) refers to, unbound
func (x Value) dotAccess(k fieldKey) (field Value) {
	if isKnown(x.pointer) {
		return Value{}
	}

	switch x.scalar {
	case kindString:
		return typeString.Fields[k.id]
	case kindArray:
		return typeArray.Fields[k.id]
	case kindBuffer:
		return typeBuffer.Fields[k.id]
	case kindTask:
		return typeTask.Fields[k.id]
	case kindObject:
		obj := *(*map[string]Value)(x.pointer)
		if value, exists := obj[k.name]; exists {
			return value
		}
		return typeObject.Fields[k.id]
	case kindError:
		return typeError.Fields[k.id]
	case kindHost:
		return x.host().typ.typ.Fields[k.id]
	case kindPackage:
		pkg := (*Package)(x.pointer)
		value := pkg.globals[k.id]
		if !value.IsPublic {
			return Value{}
		}
		return *value.Value
	case kindStructInstance:
		usi := (*UserStructInstance)(x.pointer)
		if method := usi.InstanceOf.lookupMethod(k.id); method != nil {
			return boxUserFnPtr(method)
		}
		return Value{}
	}

	// nothing else has members
	return Value{}
}

// IsCallable reports whether the value can be invoked via Call: functions,
// bound methods, and struct or builtin types, which construct a value.
func (x Value) IsCallable() bool {
	if !isKnown(x.pointer) {
		switch x.scalar {
		case kindUserFn, kindGoFunc, kindBoundMethod, kindStruct, kindBuiltinType:
			return true
		}
	}
	return false
}

func (x Value) Call(fbr *Fiber, args ...Value) (result Value, exc Exception) {
	if !isKnown(x.pointer) {
		switch x.scalar {
		case kindUserFn:
			fn := (*UserFn)(x.pointer)
			return fn.Call(fbr, args...)
		case kindGoFunc:
			fn := (*GoFunc)(x.pointer)
			return fn.Call(fbr, args...)
		case kindBoundMethod:
			bm := (*BoundMethod)(x.pointer)
			return bm.Call(fbr, args...)
		case kindStruct:
			us := (*UserStruct)(x.pointer)
			return us.instantiate(args)
		case kindBuiltinType:
			bt := (*BuiltinType)(x.pointer)
			return bt.Constructor.Call(fbr, args...)
		}
	}
	return Value{}, RuntimeExceptionF("cannot invoke a non-callable '%v'", x)
}
