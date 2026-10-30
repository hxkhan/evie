package vm

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/hxkhan/evie/vm/fields"
)

// HostType describes Go values a host lends to scripts as they are, without
// converting them into evie objects: a file handle, a database connection, a
// struct the host keeps mutating. Scripts see an opaque value with the methods
// the host registers, can test it with `x is TypeName` and construct one when
// a constructor is given. The Go type is registered, so BoxAny and the typed
// wrappers box and unbox it on their own.
type HostType struct {
	typ    *builtinType
	goType reflect.Type

	// String renders a value; the default is <TypeName>
	String func(v any) string
	// Equals compares two values; the default compares them with ==
	// when the Go type allows it and by identity otherwise
	Equals func(a, b any) bool
}

type hostValue struct {
	typ *HostType
	val any
}

var (
	hostTypesMu sync.RWMutex
	hostTypes   = map[reflect.Type]*HostType{}
)

// NewHostType registers Go values of type T under a name scripts see
func NewHostType[T any](name string) *HostType {
	t := &HostType{
		typ:    newBuiltinType(name, nil, map[fields.ID]Value{}),
		goType: reflect.TypeOf((*T)(nil)).Elem(),
	}
	hostTypesMu.Lock()
	hostTypes[t.goType] = t
	hostTypesMu.Unlock()
	return t
}

// hostTypeOf returns the registered type of a Go value
func hostTypeOf(v any) (*HostType, bool) {
	hostTypesMu.RLock()
	t, ok := hostTypes[reflect.TypeOf(v)]
	hostTypesMu.RUnlock()
	return t, ok
}

// Name returns the name scripts see
func (t *HostType) Name() string {
	return t.typ.Name
}

// Method registers a method; arity counts the arguments after the receiver,
// which the native reads with Host(0)
func (t *HostType) Method(name string, arity int, fn NativeFn) *GoFunc {
	gf := newMethod(name, arity, fn)
	t.typ.Fields[fields.Get(name)] = BoxGoFunc(gf)
	return gf
}

// Constructor makes the type callable from scripts, as in File("x")
func (t *HostType) Constructor(arity int, fn NativeFn) *GoFunc {
	t.typ.Constructor = NewFunc(t.typ.Name, arity, fn)
	return t.typ.Constructor
}

// Box returns the type itself as a value, to bind under its name so that
// scripts can construct it and test values against it
func (t *HostType) Box() Value {
	return boxBuiltinType(t.typ)
}

// Wrap boxes a Go value of this type
func (t *HostType) Wrap(v any) Value {
	return Value{scalar: kindHost, pointer: unsafe.Pointer(&hostValue{typ: t, val: v})}
}

// boxHost boxes a Go value whose type was registered with NewHostType
func boxHost(v any) (Value, bool) {
	t, ok := hostTypeOf(v)
	if !ok {
		return Value{}, false
	}
	return t.Wrap(v), true
}

// AsHost unboxes a host value
func (x Value) AsHost() (v any, t *HostType, ok bool) {
	if x.scalar != kindHost || isKnown(x.pointer) {
		return nil, nil, false
	}
	hv := (*hostValue)(x.pointer)
	return hv.val, hv.typ, true
}

func (x Value) host() *hostValue {
	return (*hostValue)(x.pointer)
}

// HostArg returns argument i as a Go value of type T
func HostArg[T any](c Call, i int) (T, error) {
	var zero T
	v, _, ok := c.Arg(i).AsHost()
	if !ok {
		return zero, c.typeError(i, hostTypeName[T]())
	}
	t, ok := v.(T)
	if !ok {
		return zero, c.typeError(i, hostTypeName[T]())
	}
	return t, nil
}

// hostTypeName is the script-facing name of a registered Go type, or the Go type
func hostTypeName[T any]() string {
	goType := reflect.TypeOf((*T)(nil)).Elem()
	hostTypesMu.RLock()
	t, ok := hostTypes[goType]
	hostTypesMu.RUnlock()
	if ok {
		return "a " + t.typ.Name
	}
	return fmt.Sprintf("a %v", goType)
}

func (hv *hostValue) String() string {
	if hv.typ.String != nil {
		return hv.typ.String(hv.val)
	}
	return "<" + hv.typ.Name() + ">"
}

func (hv *hostValue) equals(other *hostValue) bool {
	if hv.typ != other.typ {
		return false
	}
	if hv.typ.Equals != nil {
		return hv.typ.Equals(hv.val, other.val)
	}
	if hv.typ.goType.Comparable() {
		return hv.val == other.val
	}
	return hv == other
}
