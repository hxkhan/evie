package vm

import (
	"fmt"
	"unsafe"

	"github.com/hxkhan/evie/vm/fields"
)

// Package is a namespace of symbols: the globals of a compiled evie file, or
// the functions and constants a host registers for scripts to import.
type Package struct {
	name    string                  // name of the package
	globals map[fields.ID]pkgGlobal // all global symbols

	origin     string               // the file it was compiled from, for errors
	importedAs map[string][2]string // path name -> (import path, bound name) where they differ
	memberHint string               // shown when a script asks for a member it lacks
}

// SetMemberHint sets the help shown when a script asks the package for a
// member it does not have, such as which import to use instead
func (pkg *Package) SetMemberHint(hint string) {
	pkg.memberHint = hint
}

// NewPackage creates an empty host package. Scripts import it by the name it
// is resolved under; an empty name is filled in from the import path.
func NewPackage(name string) *Package {
	return &Package{name: name, globals: map[fields.ID]pkgGlobal{}}
}

func (pkg *Package) Name() string {
	return pkg.name
}

func (pkg *Package) setName(name string) {
	pkg.name = name
}

// Box boxes the package to be used as a value
func (pkg *Package) Box() Value {
	return Value{scalar: kindPackage, pointer: unsafe.Pointer(pkg)}
}

// Set binds a public constant symbol. Setting an existing symbol replaces its
// value in place, so compiled code that refers to it sees the new value.
func (pkg *Package) Set(name string, value Value) *Package {
	index := fields.Get(name)
	if ref, exists := pkg.globals[index]; exists {
		*ref.Value = value
	} else {
		pkg.globals[index] = pkgGlobal{Value: &value, IsPublic: true, IsStatic: true, IsConstant: true}
	}
	return pkg
}

// Func binds a native taking exactly arity arguments and returns it so that
// flags such as Blocking can be set
func (pkg *Package) Func(name string, arity int, fn NativeFn) *GoFunc {
	gf := NewFunc(name, arity, fn)
	pkg.Set(name, BoxGoFunc(gf))
	return gf
}

// Variadic binds a native taking at least min arguments
func (pkg *Package) Variadic(name string, min int, fn NativeFn) *GoFunc {
	gf := NewVariadic(name, min, fn)
	pkg.Set(name, BoxGoFunc(gf))
	return gf
}

// Get returns a symbol's value
func (pkg *Package) Get(name string) (value Value, exists bool) {
	v, exists := pkg.globals[fields.Get(name)]
	if !exists || v.Value == nil {
		return Value{}, false
	}
	return *v.Value, true
}

// Has reports whether a symbol exists
func (pkg *Package) Has(name string) bool {
	_, exists := pkg.globals[fields.Get(name)]
	return exists
}

// The operators as scripts apply them, for hosts whose natives should behave
// like the language: + adds numbers and joins strings, the others work on
// numbers. Operands of the wrong types give the error a script would get,
// such as "cannot apply '+' to a number and a string".

// Add applies + to x and y
func (x Value) Add(y Value) (Value, error) {
	if z, ok := x.add(y); ok {
		return z, nil
	}
	return Value{}, operatorError(unknownSite, "+", x, y)
}

// Sub applies - to x and y
func (x Value) Sub(y Value) (Value, error) {
	if z, ok := x.sub(y); ok {
		return z, nil
	}
	return Value{}, operatorError(unknownSite, "-", x, y)
}

// Mul applies * to x and y
func (x Value) Mul(y Value) (Value, error) {
	if z, ok := x.mul(y); ok {
		return z, nil
	}
	return Value{}, operatorError(unknownSite, "*", x, y)
}

// Div applies / to x and y
func (x Value) Div(y Value) (Value, error) {
	if z, ok := x.div(y); ok {
		return z, nil
	}
	return Value{}, operatorError(unknownSite, "/", x, y)
}

// Mod applies % to x and y
func (x Value) Mod(y Value) (Value, error) {
	if z, ok := x.mod(y); ok {
		return z, nil
	}
	return Value{}, operatorError(unknownSite, "%", x, y)
}

// LessThan applies < to x and y
func (x Value) LessThan(y Value) (bool, error) {
	z, ok := x.lessThan(y)
	return compared(z, ok, x, y, "<")
}

// GreaterThan applies > to x and y
func (x Value) GreaterThan(y Value) (bool, error) {
	z, ok := x.greaterThan(y)
	return compared(z, ok, x, y, ">")
}

// LessThanOrEqualTo applies <= to x and y
func (x Value) LessThanOrEqualTo(y Value) (bool, error) {
	z, ok := x.lessThanOrEqualTo(y)
	return compared(z, ok, x, y, "<=")
}

// GreaterThanOrEqualTo applies >= to x and y
func (x Value) GreaterThanOrEqualTo(y Value) (bool, error) {
	z, ok := x.greaterThanOrEqualTo(y)
	return compared(z, ok, x, y, ">=")
}

// compared turns an internal comparison result into a Go bool, or the error
// for operands of the wrong types
func compared(z Value, ok bool, x, y Value, op string) (bool, error) {
	if !ok {
		return false, operatorError(unknownSite, op, x, y)
	}
	b, _ := z.AsBool()
	return b, nil
}

// Field reads a field of an object or of a struct instance by name
func (x Value) Field(name string) (Value, bool) {
	if obj, ok := x.AsObject(); ok {
		v, exists := obj[name]
		return v, exists
	}
	if usi, ok := x.asStructInstance(); ok {
		return usi.getFieldByName(name)
	}
	return Value{}, false
}

// SetField sets a field of an object or of a struct instance by name; a name
// a struct does not declare becomes a dynamic field, as in scripts
func (x Value) SetField(name string, v Value) error {
	if obj, ok := x.AsObject(); ok {
		obj[name] = v
		return nil
	}
	if usi, ok := x.asStructInstance(); ok {
		usi.setFieldByName(name, v)
		return nil
	}
	return fmt.Errorf("cannot set field '%s' on %s", name, article(x))
}

// StructName is the name of the struct a struct instance was made from
func (x Value) StructName() (string, bool) {
	if usi, ok := x.asStructInstance(); ok {
		return usi.InstanceOf.Name, true
	}
	return "", false
}
