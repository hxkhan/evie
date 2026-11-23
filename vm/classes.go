package vm

import (
	"strings"
	"unsafe"

	"github.com/hxkhan/evie/diag"
	"github.com/hxkhan/evie/internal/fields"
)

type userClass struct {
	Name         string
	def          site
	Parent       *userClass
	ParamCount   int               // required constructor arguments (Fields[:ParamCount])
	Fields       []fields.ID       // fixed layout: [0..ParamCount-1] constructor args, [ParamCount..] body & inherited fields
	StaticFields []bool            // parallel to Fields: true for immutable (:=) body fields
	Defaults     []Value           // parallel to Fields[ParamCount:]: constant default values
	FieldInit    *userFn           // initializer for this class's own non-constant body fields (nil if all constant)
	FieldInits   []*userFn         // ancestor-first chain of non-nil FieldInit functions to run on construction
	FieldOffsets map[fields.ID]int // field id -> fixed slot index
	Methods      map[fields.ID]*userFn
	declClosure  *closure // non-nil only for local classes with a runtime-resolved parent
	declIndex    int
	decls        map[fields.ID]nameDecl // field or method -> where it is declared, when names are recorded
}

func (uc *userClass) hasParent() bool {
	return uc.Parent != nil || uc.declClosure != nil
}

func newUserClass(name string, def site, params []string) *userClass {
	fids := make([]fields.ID, len(params))
	offsets := make(map[fields.ID]int, len(params))
	for i, a := range params {
		fid := fields.Get(a)
		fids[i] = fid
		offsets[fid] = i
	}
	return &userClass{
		Name:         name,
		def:          def,
		ParamCount:   len(params),
		Fields:       fids,
		StaticFields: make([]bool, len(params)),
		FieldOffsets: offsets,
		Methods:      make(map[fields.ID]*userFn),
	}
}

// instantiate creates an instance from pre-evaluated constructor arguments (e.g. from Go Call).
func (uc *userClass) instantiate(fbr *fiber, args []Value) (Value, exception) {
	if len(args) != uc.ParamCount {
		return Value{}, customError("class '%v' takes %v argument(s) but %v were supplied", uc.Name, uc.ParamCount, len(args))
	}
	ci := &classInstance{InstanceOf: uc, Fields: make([]Value, len(uc.Fields))}
	copy(ci.Fields[:uc.ParamCount], args)
	if len(uc.Defaults) > 0 {
		copy(ci.Fields[uc.ParamCount:], uc.Defaults)
	}
	boxed := boxClassInstance(ci)
	for _, initFn := range uc.FieldInits {
		if _, exc := initFn.callGuarded(fbr, boxed); exc != nil {
			return Value{}, exc
		}
	}
	return boxed, nil
}

// construct evaluates the constructor arguments and creates an instance.
func (uc *userClass) construct(fbr *fiber, site site, arguments []instruction) (Value, exception) {
	if len(arguments) != uc.ParamCount {
		return Value{}, errorAt(site, "class '%v' takes %v argument(s) but %v were supplied", uc.Name, uc.ParamCount, len(arguments))
	}
	ci := &classInstance{InstanceOf: uc, Fields: make([]Value, len(uc.Fields))}
	for i, argument := range arguments {
		v, exc := argument(fbr)
		if exc != nil {
			return v, exc
		}
		ci.Fields[i] = v
	}
	if len(uc.Defaults) > 0 {
		copy(ci.Fields[uc.ParamCount:], uc.Defaults)
	}
	boxed := boxClassInstance(ci)
	if len(uc.FieldInits) > 0 {
		var argBuf [1]Value
		argBuf[0] = boxed
		for _, initFn := range uc.FieldInits {
			if _, exc := initFn.call(fbr, argBuf[:]); exc != nil {
				return Value{}, exc.unwind(site)
			}
		}
	}
	return boxed, nil
}

// lookupMethod finds a method on the class or one of its ancestors
func (uc *userClass) lookupMethod(f fields.ID) *userFn {
	for cur := uc; cur != nil; cur = cur.Parent {
		if fn, ok := cur.Methods[f]; ok {
			return fn
		}
	}
	return nil
}

// slot returns the index of a declared field, looked up by its interned id
func (uc *userClass) slot(f fields.ID) (int, bool) {
	slot, ok := uc.FieldOffsets[f]
	return slot, ok
}

// slotByName returns the index of a declared field, looked up by name without
// interning it: a name that was never interned cannot be a declared field
func (uc *userClass) slotByName(name string) (int, bool) {
	if f, ok := fields.Find(name); ok {
		return uc.slot(f)
	}
	return 0, false
}

func (uc *userClass) fieldNames() []string {
	names := make([]string, len(uc.Fields))
	for i, fid := range uc.Fields {
		names[i] = fields.Lookup(fid)
	}
	return names
}

type classInstance struct {
	InstanceOf *userClass
	Fields     []Value // fixed-size slice matching InstanceOf.Fields
}

// GetField reads a field named by a literal in the source
func (ci *classInstance) GetField(f fields.ID) (Value, bool) {
	if slot, ok := ci.InstanceOf.slot(f); ok && slot < len(ci.Fields) {
		return ci.Fields[slot], true
	}
	return Value{}, false
}

// getFieldByName reads a field named by a runtime string
func (ci *classInstance) getFieldByName(name string) (Value, bool) {
	if slot, ok := ci.InstanceOf.slotByName(name); ok && slot < len(ci.Fields) {
		return ci.Fields[slot], true
	}
	return Value{}, false
}

func (ci *classInstance) missingFieldError(site site, name string) exception {
	err := errorAt(site, "class '%s' has no field '%s'", ci.InstanceOf.Name, name)
	if hint, ok := diag.Suggest(name, ci.InstanceOf.fieldNames()); ok {
		err = err.hinted("did you mean '" + hint + "'?")
	}
	return err
}

func (ci *classInstance) constFieldError(site site, name string) exception {
	return errorAt(site, "cannot assign to constant field '%s' on class '%s'", name, ci.InstanceOf.Name).
		hinted("declare it with var to allow reassignment: var " + name + " := ...")
}

// SetField writes a declared mutable field named by a literal in the source.
func (ci *classInstance) SetField(site site, f fields.ID, name string, val Value) exception {
	slot, ok := ci.InstanceOf.slot(f)
	if !ok || slot >= len(ci.Fields) {
		return ci.missingFieldError(site, name)
	}
	if ci.InstanceOf.StaticFields[slot] {
		return ci.constFieldError(site, name)
	}
	ci.Fields[slot] = val
	return nil
}

// setFieldByName writes a declared mutable field named by a runtime string.
func (ci *classInstance) setFieldByName(site site, name string, val Value) exception {
	slot, ok := ci.InstanceOf.slotByName(name)
	if !ok || slot >= len(ci.Fields) {
		return ci.missingFieldError(site, name)
	}
	if ci.InstanceOf.StaticFields[slot] {
		return ci.constFieldError(site, name)
	}
	ci.Fields[slot] = val
	return nil
}

// UpdateField replaces a declared mutable field named by a literal with update(current).
func (ci *classInstance) UpdateField(site site, f fields.ID, name string, update func(Value) (Value, exception)) exception {
	slot, ok := ci.InstanceOf.slot(f)
	if !ok || slot >= len(ci.Fields) {
		return ci.missingFieldError(site, name)
	}
	if ci.InstanceOf.StaticFields[slot] {
		return ci.constFieldError(site, name)
	}
	value, exc := update(ci.Fields[slot])
	if exc == nil {
		ci.Fields[slot] = value
	}
	return exc
}

// updateFieldByName replaces a declared mutable field named by a runtime string with update(current).
func (ci *classInstance) updateFieldByName(site site, name string, update func(Value) (Value, exception)) exception {
	slot, ok := ci.InstanceOf.slotByName(name)
	if !ok || slot >= len(ci.Fields) {
		return ci.missingFieldError(site, name)
	}
	if ci.InstanceOf.StaticFields[slot] {
		return ci.constFieldError(site, name)
	}
	value, exc := update(ci.Fields[slot])
	if exc == nil {
		ci.Fields[slot] = value
	}
	return exc
}

// HasField reports whether a declared field named by a literal exists
func (ci *classInstance) HasField(f fields.ID) bool {
	_, ok := ci.InstanceOf.slot(f)
	return ok
}

// hasFieldByName reports whether a declared field named by a runtime string exists
func (ci *classInstance) hasFieldByName(name string) bool {
	_, ok := ci.InstanceOf.slotByName(name)
	return ok
}

func (ci *classInstance) String() string {
	var builder strings.Builder
	ci.writeTo(&builder, nil)
	return builder.String()
}

func (ci *classInstance) writeTo(builder *strings.Builder, seen map[unsafe.Pointer]bool) {
	if ci == nil {
		builder.WriteString("{}")
		return
	}
	ptr := unsafe.Pointer(ci)
	if seen[ptr] {
		builder.WriteString(ci.InstanceOf.Name)
		builder.WriteString("{...}")
		return
	}
	if seen == nil {
		seen = make(map[unsafe.Pointer]bool)
	}
	seen[ptr] = true
	defer delete(seen, ptr)

	builder.WriteString(ci.InstanceOf.Name)
	builder.WriteByte('{')

	n := min(len(ci.InstanceOf.Fields), len(ci.Fields))
	for i := range n {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(fields.Lookup(ci.InstanceOf.Fields[i]))
		builder.WriteString(": ")
		ci.Fields[i].writeTo(builder, seen, true)
	}

	builder.WriteByte('}')
}
