package vm

import (
	"maps"
	"slices"
	"unsafe"

	"github.com/hxkhan/evie/internal/fields"
)

// Package is a namespace of symbols: the globals of a compiled evie file, or
// the functions and constants a host registers for scripts to import.
type Package struct {
	id      string                  // canonical module id (unique per loaded module; defaults to name)
	name    string                  // declared package name
	globals map[fields.ID]pkgGlobal // all global symbols

	origin     string                 // the file it was compiled from, for errors
	compiled   bool                   // true for Evie packages compiled from source
	inits      []*funcInfoStatic      // package init functions recorded when Options.DebugInfo is set
	imports    []string               // formatted import bindings for Dump (e.g. "math", "./math as mymath")
	importedAs map[string][2]string   // path/declared name -> (import path, bound name) where they differ
	memberHint string                 // shown when a script asks for a member it lacks
	docs       map[string][2]string   // member name -> signature and description, for tools
	decls      map[fields.ID]nameDecl // global -> where it is declared, when names are recorded
}

// SetMemberHint sets the help shown when a script asks the package for a
// member it does not have, such as which import to use instead
func (pkg *Package) SetMemberHint(hint string) {
	pkg.memberHint = hint
}

// NewPackage creates an empty host package. Scripts import it by the name it
// is resolved under; an empty name is filled in from the import path.
func NewPackage(name string) *Package {
	return &Package{id: name, name: name, globals: map[fields.ID]pkgGlobal{}}
}

// Clone returns an independent copy of the package's namespace so an engine can
// attach per-engine hints or prelude builtins without mutating a package shared
// with the caller or other engines.
func (pkg *Package) Clone() *Package {
	cp := &Package{
		id:      pkg.id,
		name:    pkg.name,
		globals: make(map[fields.ID]pkgGlobal, len(pkg.globals)),
		origin:  pkg.origin,
	}
	for id, g := range pkg.globals {
		// each global gets its own cell, so setting it in the clone leaves the
		// original and every other clone alone
		if g.Value != nil {
			v := *g.Value
			g.Value = &v
		}
		cp.globals[id] = g
	}
	cp.memberHint = pkg.memberHint
	cp.docs = maps.Clone(pkg.docs)
	return cp
}

// ID returns the package's canonical module identifier (falling back to Name).
func (pkg *Package) ID() string {
	if pkg.id != "" {
		return pkg.id
	}
	return pkg.name
}

// SetID sets the package's canonical module identifier.
func (pkg *Package) SetID(id string) {
	pkg.id = id
}

// Name returns the package's declared name
func (pkg *Package) Name() string {
	return pkg.name
}

// Compiled reports whether the package was compiled from Evie source, rather
// than made by a host in Go
func (pkg *Package) Compiled() bool {
	return pkg.compiled
}

func (pkg *Package) setName(name string) {
	pkg.name = name
	if pkg.id == "" {
		pkg.id = name
	}
}

// Box boxes the package to be used as a value
func (pkg *Package) Box() Value {
	return Value{scalar: kindPackage, pointer: unsafe.Pointer(pkg)}
}

// Set binds a public read-only symbol on the package. Immutable values such as
// numbers, strings, functions and types may be folded by the compiler; mutable
// containers such as objects, arrays and buffers are read dynamically so later
// mutations are always visible.
func (pkg *Package) Set(name string, value Value) *Package {
	index := fields.Get(name)
	if ref, exists := pkg.globals[index]; exists {
		*ref.Value = value
		ref.IsConstant = false
		pkg.globals[index] = ref
	} else {
		pkg.globals[index] = pkgGlobal{Value: &value, IsPublic: true, IsStatic: true, IsConstant: value.isImmutableConstant()}
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
	id, known := fields.Find(name) // a lookup must not intern arbitrary host strings
	if !known {
		return Value{}, false
	}
	v, exists := pkg.globals[id]
	if !exists || v.Value == nil {
		return Value{}, false
	}
	return *v.Value, true
}

// Has reports whether a symbol exists
func (pkg *Package) Has(name string) bool {
	id, known := fields.Find(name)
	if !known {
		return false
	}
	_, exists := pkg.globals[id]
	return exists
}

// Member describes a public member of a package for tools such as editors
type Member struct {
	Name string
	// Signature shows how to call it, as in "readFile(path)"; for anything that
	// is not a function it is the name
	Signature string
	Doc       string // what it does, when documented
	// Yields reports that calling it waits on the outside world, such as a file
	// or a timer, and lets the instance's other tasks run in the meantime
	Yields bool
	Value  Value
}

// Document describes a member, for tools; it does not change what the member
// does. signature shows how to call it, as in "readFile(path)".
func (pkg *Package) Document(name, signature, doc string) *Package {
	if pkg.docs == nil {
		pkg.docs = map[string][2]string{}
	}
	pkg.docs[name] = [2]string{signature, doc}
	return pkg
}

// Members returns the names of the package's public members, sorted
func (pkg *Package) Members() []string {
	var names []string
	for id, g := range pkg.globals {
		if g.IsPublic {
			names = append(names, fields.Lookup(id))
		}
	}
	slices.Sort(names)
	return names
}

// Describe tells tools about a public member: how to call it, what it does
// and whether calling it yields
func (pkg *Package) Describe(name string) (Member, bool) {
	id, known := fields.Find(name)
	if !known {
		return Member{}, false
	}
	g, exists := pkg.globals[id]
	if !exists || !g.IsPublic || g.Value == nil {
		return Member{}, false
	}
	m := Member{Name: name, Signature: name, Value: *g.Value}
	if fn, ok := m.Value.AsGoFunc(); ok {
		m.Yields = fn.Blocking
	}
	if doc, ok := pkg.docs[name]; ok {
		m.Signature, m.Doc = doc[0], doc[1]
	}
	return m, true
}

// Field reads a field of an object or of a class instance by name
func (x Value) Field(name string) (Value, bool) {
	if obj, ok := x.AsObject(); ok {
		return obj.Get(name)
	}
	if ci, ok := x.asClassInstance(); ok {
		return ci.getFieldByName(name)
	}
	return Value{}, false
}

// SetField sets a field of an object or a declared mutable field of a class
// instance by name.
func (x Value) SetField(name string, v Value) error {
	if obj, ok := x.AsObject(); ok {
		obj.Set(name, v)
		return nil
	}
	if ci, ok := x.asClassInstance(); ok {
		if exc := ci.setFieldByName(unknownSite, name, v); exc != nil {
			return exc
		}
		return nil
	}
	return Errorf(KindType, "cannot set field '%s' on %s", name, article(x))
}

// ClassName is the name of the class a class instance was made from
func (x Value) ClassName() (string, bool) {
	if ci, ok := x.asClassInstance(); ok {
		return ci.InstanceOf.Name, true
	}
	return "", false
}
