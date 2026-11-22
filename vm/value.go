package vm

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unsafe"

	"github.com/hxkhan/evie/internal/fields"
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
	kindClass
	kindClassInstance
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

// boxUserFn boxes an evie function
func boxUserFn(fn userFn) Value {
	return Value{scalar: kindUserFn, pointer: unsafe.Pointer(&fn)}
}

// boxUserFnPtr boxes an existing function without copying it
func boxUserFnPtr(fn *userFn) Value {
	return Value{scalar: kindUserFn, pointer: unsafe.Pointer(fn)}
}

// boxBuiltinType boxes a Go function that is a type constructor
func boxBuiltinType(t *builtinType) Value {
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

// BoxObject boxes an evie object; nil boxes a new empty one
func BoxObject(obj *Object) Value {
	if obj == nil {
		obj = NewObject(0)
	}
	return Value{scalar: kindObject, pointer: unsafe.Pointer(obj)}
}

// object is the Object of a value that holds one
func (x Value) object() *Object {
	return (*Object)(x.pointer)
}

// boxUserClass boxes an evie class
func boxUserClass(obj *userClass) Value {
	return Value{scalar: kindClass, pointer: unsafe.Pointer(obj)}
}

// boxClassInstance boxes an evie class instance
func boxClassInstance(obj *classInstance) Value {
	return Value{scalar: kindClassInstance, pointer: unsafe.Pointer(obj)}
}

// boxTask boxes an evie task
func boxTask(task *Task) Value {
	return Value{scalar: kindTask, pointer: unsafe.Pointer(task)}
}

// boxError boxes an evie error
func boxError(err *Error) Value {
	return Value{scalar: kindError, pointer: unsafe.Pointer(err)}
}

func boxMethod(m boundMethod) Value {
	return Value{scalar: kindBoundMethod, pointer: unsafe.Pointer(&m)}
}

// BoxBuffer boxes a Golang byte slice
func BoxBuffer(bytes []byte) Value {
	return Value{scalar: kindBuffer, pointer: unsafe.Pointer(&bytes)}
}

func (x Value) IsNil() bool {
	return x.pointer == nil
}

// isImmutableConstant reports whether a value can safely be folded into
// compiled code as a constant without freezing mutable state
func (x Value) isImmutableConstant() bool {
	if isKnown(x.pointer) {
		return true
	}
	switch x.scalar {
	case kindString, kindUserFn, kindGoFunc, kindClass, kindBuiltinType, kindPackage:
		return true
	}
	return false
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

func (x Value) asUserFn() (fn *userFn, ok bool) {
	if x.scalar != kindUserFn || isKnown(x.pointer) {
		return nil, false
	}
	return (*userFn)(x.pointer), true
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

func (x Value) AsObject() (obj *Object, ok bool) {
	if x.scalar != kindObject || isKnown(x.pointer) {
		return nil, false
	}
	return x.object(), true
}

func (x Value) asUserClass() (obj *userClass, ok bool) {
	if x.scalar != kindClass || isKnown(x.pointer) {
		return nil, false
	}
	return (*userClass)(x.pointer), true
}

func (x Value) asClassInstance() (obj *classInstance, ok bool) {
	if x.scalar != kindClassInstance || isKnown(x.pointer) {
		return nil, false
	}
	return (*classInstance)(x.pointer), true
}

func (x Value) asBuiltinType() (obj *builtinType, ok bool) {
	if x.scalar != kindBuiltinType || isKnown(x.pointer) {
		return nil, false
	}
	return (*builtinType)(x.pointer), true
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

func (x Value) asMethod() (m *boundMethod, ok bool) {
	if x.scalar != kindBoundMethod || isKnown(x.pointer) {
		return nil, false
	}
	return (*boundMethod)(x.pointer), true
}

func (x Value) AsBuffer() (buffer []byte, ok bool) {
	if x.scalar != kindBuffer || isKnown(x.pointer) {
		return nil, false
	}
	return *(*[]byte)(x.pointer), true
}

// allocate will copy the current value to the heap and return a pointer to it
func (x Value) allocate() *Value {
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
	case kindArray:
		return len((*Array)(x.pointer).Data) != 0
	case kindObject:
		return x.object().Len() != 0
	case kindBuffer:
		return len(*(*[]byte)(x.pointer)) != 0
	default:
		// functions, bound methods, class types and instances, tasks, errors,
		// packages, builtin types and host values are truthy
		return true
	}
}

func (x Value) Equals(y Value) bool {
	if x.pointer == f64Type && y.pointer == f64Type {
		return math.Float64frombits(x.scalar) == math.Float64frombits(y.scalar)
	}
	return x.equalsSlow(y)
}

func (x Value) equalsSlow(y Value) bool {
	switch x.pointer {
	case nil:
		return y.pointer == nil
	case boolType:
		return y.pointer == boolType && x.scalar == y.scalar
	case f64Type:
		return false
	}

	// guarantees that both are reference values of the same kind beyond this point
	if x.scalar != y.scalar || isKnown(y.pointer) {
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
	case kindClassInstance:
		var builder strings.Builder
		(*classInstance)(x.pointer).writeTo(&builder, nil)
		return builder.String()
	case kindGoFunc:
		fn := (*GoFunc)(x.pointer)
		if fn.isMethod {
			return "<callable>"
		}
		return "<callable>"

	case kindArray:
		var builder strings.Builder
		(*Array)(x.pointer).writeTo(&builder, nil)
		return builder.String()

	case kindObject:
		var builder strings.Builder
		x.writeTo(&builder, nil, false)
		return builder.String()

	case kindClass:
		obj := (*userClass)(x.pointer)
		return fmt.Sprintf("<type %v>", obj.Name)
	case kindTask:
		t := (*Task)(x.pointer)
		if t.isDone() {
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
		obj := (*builtinType)(x.pointer)
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

func (x Value) writeTo(builder *strings.Builder, seen map[unsafe.Pointer]bool, quoteStrings bool) {
	if !isKnown(x.pointer) {
		switch x.scalar {
		case kindString:
			s := *(*string)(x.pointer)
			if quoteStrings {
				builder.WriteByte('"')
				builder.WriteString(s)
				builder.WriteByte('"')
			} else {
				builder.WriteString(s)
			}
			return
		case kindArray:
			(*Array)(x.pointer).writeTo(builder, seen)
			return
		case kindClassInstance:
			(*classInstance)(x.pointer).writeTo(builder, seen)
			return
		case kindObject:
			obj := x.object()
			ptr := x.pointer
			if ptr != nil && seen[ptr] {
				builder.WriteString("{...}")
				return
			}
			if seen == nil {
				seen = make(map[unsafe.Pointer]bool)
			}
			if ptr != nil {
				seen[ptr] = true
				defer delete(seen, ptr)
			}
			builder.WriteByte('{')
			first := true
			for name, value := range obj.All() {
				if !first {
					builder.WriteString(", ")
				}
				first = false
				builder.WriteString(name)
				builder.WriteString(": ")
				value.writeTo(builder, seen, true)
			}
			builder.WriteByte('}')
			return
		}
	}
	builder.WriteString(x.String())
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
	case kindClass:
		return "class"
	case kindClassInstance:
		return "class"
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
// are keyed by the name itself; classes, packages and builtin types by the
// interned id, which the compiler resolves once.
type fieldKey struct {
	id   fields.ID
	name string
}

// fieldKeyOf names a member; the name is the interned one, so object keys from
// literals compare with it by pointer
func fieldKeyOf(name string) fieldKey {
	id := fields.Get(name)
	return fieldKey{id: id, name: fields.Lookup(id)}
}

var (
	messageID = fields.Get("message")
	lineID    = fields.Get("line")
	columnID  = fields.Get("column")
	fileID    = fields.Get("file")
	traceID   = fields.Get("trace")
)

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
		if value, exists := x.object().Get(k.name); exists {
			return value, true
		}
		return x.boundMethod(typeObject, k.id)

	case kindBuiltinType:
		obj := (*builtinType)(x.pointer)
		value, exists := obj.Fields[k.id]
		return value, exists

	case kindClassInstance:
		obj := (*classInstance)(x.pointer)
		if value, exists := obj.GetField(k.id); exists {
			return value, true
		}
		if method := obj.InstanceOf.lookupMethod(k.id); method != nil {
			return boxMethod(boundMethod{this: x, fn: boxUserFnPtr(method)}), true
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
		return append(x.object().Keys(), ids(typeObject.Fields)...)
	case kindBuiltinType:
		return ids((*builtinType)(x.pointer).Fields)
	case kindHost:
		return ids(x.host().typ.typ.Fields)
	case kindPackage:
		var names []string
		for id, g := range (*Package)(x.pointer).globals {
			if g.IsPublic {
				names = append(names, fields.Lookup(id))
			}
		}
		return names
	case kindClassInstance:
		ci := (*classInstance)(x.pointer)
		var names []string
		for cur := ci.InstanceOf; cur != nil; cur = cur.Parent {
			for _, f := range cur.Fields {
				names = append(names, fields.Lookup(f))
			}
			for id := range cur.Methods {
				names = append(names, fields.Lookup(id))
			}
		}
		return names
	}
	return nil
}

// boundMethod looks a method up on a builtin type and binds it to x
func (x Value) boundMethod(t *builtinType, f fields.ID) (Value, bool) {
	value, exists := t.Fields[f]
	if !exists {
		return Value{}, false
	}
	return boxMethod(boundMethod{this: x, fn: value}), true
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
		if value, exists := x.object().Get(k.name); exists {
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
	case kindClassInstance:
		ci := (*classInstance)(x.pointer)
		if method := ci.InstanceOf.lookupMethod(k.id); method != nil {
			return boxUserFnPtr(method)
		}
		return Value{}
	}

	// nothing else has members
	return Value{}
}

// IsCallable reports whether the value can be invoked via Call: functions,
// bound methods, and class or builtin types, which construct a value.
func (x Value) IsCallable() bool {
	if !isKnown(x.pointer) {
		switch x.scalar {
		case kindUserFn, kindGoFunc, kindBoundMethod, kindClass:
			return true
		case kindBuiltinType:
			return (*builtinType)(x.pointer).Constructor != nil
		}
	}
	return false
}

// AcceptsArgs reports whether a callable value accepts one or more arguments
func (x Value) AcceptsArgs() bool {
	if !isKnown(x.pointer) {
		switch x.scalar {
		case kindUserFn:
			return (*userFn)(x.pointer).maxArgs > 0
		case kindGoFunc:
			fn := (*GoFunc)(x.pointer)
			return fn.isVariadic || fn.arguments > 0
		case kindBoundMethod:
			bm := (*boundMethod)(x.pointer)
			if fn, ok := bm.fn.asUserFn(); ok {
				if fn.isMethod {
					return fn.maxArgs > 1
				}
				return fn.maxArgs > 0
			}
			if fn, ok := bm.fn.AsGoFunc(); ok {
				if fn.isMethod {
					return fn.isVariadic || fn.arguments > 1
				}
				return fn.isVariadic || fn.arguments > 0
			}
			return bm.fn.AcceptsArgs()
		case kindClass:
			return (*userClass)(x.pointer).ParamCount > 0
		case kindBuiltinType:
			fn := (*builtinType)(x.pointer).Constructor
			return fn != nil && (fn.isVariadic || fn.arguments > 0)
		}
	}
	return false
}

// Call calls a callable value from Go: a script function, a native, a bound
// method or a type. It borrows a fiber of the instance the value belongs to.
func (x Value) Call(args ...Value) (Value, error) {
	v, exc := x.callGuarded(nil, args...)
	if exc != nil {
		return v, exc
	}
	return v, nil
}

// callGuarded calls a callable value on fbr, or on a borrowed fiber when fbr
// is nil, recovering panics at the boundary
func (x Value) callGuarded(fbr *fiber, args ...Value) (result Value, exc exception) {
	if !isKnown(x.pointer) {
		switch x.scalar {
		case kindUserFn:
			fn := (*userFn)(x.pointer)
			return fn.callGuarded(fbr, args...)
		case kindGoFunc:
			fn := (*GoFunc)(x.pointer)
			return fn.callGuarded(fbr, args...)
		case kindBoundMethod:
			bm := (*boundMethod)(x.pointer)
			return bm.callGuarded(fbr, args...)
		case kindClass:
			uc := (*userClass)(x.pointer)
			return uc.instantiate(fbr, args)
		case kindBuiltinType:
			bt := (*builtinType)(x.pointer)
			if bt.Constructor == nil {
				return Value{}, runtimeErrorf("type '%s' cannot be called as a constructor", bt.Name)
			}
			return bt.Constructor.callGuarded(fbr, args...)
		}
	}
	return Value{}, runtimeErrorf("cannot invoke a non-callable '%v'", x)
}
