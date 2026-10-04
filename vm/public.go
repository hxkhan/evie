package vm

import (
	"unsafe"

	"github.com/hxkhan/evie/vm/fields"
)

// Package is a namespace of symbols: the globals of a compiled evie file, or
// the functions and constants a host registers for scripts to import.
type Package struct {
	name    string               // name of the package
	globals map[fields.ID]Global // all global symbols

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
	return &Package{name: name, globals: map[fields.ID]Global{}}
}

func (pkg *Package) Name() string {
	return pkg.name
}

func (pkg *Package) SetName(name string) {
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
		pkg.globals[index] = Global{Value: &value, IsPublic: true, IsStatic: true, IsConstant: true}
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
