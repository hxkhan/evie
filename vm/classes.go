package vm

import (
	"strings"
	"unsafe"

	"github.com/hxkhan/evie/internal/fields"
)

type userClass struct {
	Name         string
	def          site
	hasParent    bool
	Parent       *userClass
	Fields       []fields.ID
	FieldOffsets map[fields.ID]int
	Methods      map[fields.ID]*userFn
	declClosure  *closure // non-nil only for local classes with a runtime-resolved parent
	declIndex    int
}

// instantiate creates an instance from the constructor arguments
func (uc *userClass) instantiate(args []Value) (Value, exception) {
	if len(args) != len(uc.Fields) {
		return Value{}, customError("class '%v' takes %v argument(s) but %v were supplied", uc.Name, len(uc.Fields), len(args))
	}
	fieldVals := make([]Value, len(args))
	copy(fieldVals, args)
	return boxClassInstance(&classInstance{InstanceOf: uc, Fields: fieldVals}), nil
}

// construct evaluates the constructor arguments and creates an instance
func (uc *userClass) construct(fbr *fiber, site site, arguments []instruction) (Value, exception) {
	if len(arguments) != len(uc.Fields) {
		return Value{}, errorAt(site, "class '%v' takes %v argument(s) but %v were supplied", uc.Name, len(uc.Fields), len(arguments))
	}
	ci := &classInstance{InstanceOf: uc, Fields: make([]Value, len(arguments))}
	for i, argument := range arguments {
		v, exc := argument(fbr)
		if exc != nil {
			return v, exc
		}
		ci.Fields[i] = v
	}
	return boxClassInstance(ci), nil
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

type classInstance struct {
	InstanceOf *userClass
	Fields     []Value          // Fixed array for declared fields: [0]=arg0, [1]=arg1
	Dynamic    map[string]Value // Lazily allocated ONLY if ad-hoc dynamic fields are added
}

// GetField reads a field named by a literal in the source
func (ci *classInstance) GetField(f fields.ID) (Value, bool) {
	if slot, ok := ci.InstanceOf.slot(f); ok && slot < len(ci.Fields) {
		return ci.Fields[slot], true
	}
	if ci.Dynamic != nil {
		val, exists := ci.Dynamic[fields.Lookup(f)]
		return val, exists
	}
	return Value{}, false
}

// getFieldByName reads a field named by a runtime string
func (ci *classInstance) getFieldByName(name string) (Value, bool) {
	if slot, ok := ci.InstanceOf.slotByName(name); ok && slot < len(ci.Fields) {
		return ci.Fields[slot], true
	}
	if ci.Dynamic != nil {
		val, exists := ci.Dynamic[name]
		return val, exists
	}
	return Value{}, false
}

// SetField writes a field named by a literal in the source, adding it if it is not declared
func (ci *classInstance) SetField(f fields.ID, val Value) {
	if slot, ok := ci.InstanceOf.slot(f); ok && slot < len(ci.Fields) {
		ci.Fields[slot] = val
		return
	}
	ci.setDynamic(fields.Lookup(f), val)
}

// setFieldByName writes a field named by a runtime string, adding it if it is not declared
func (ci *classInstance) setFieldByName(name string, val Value) {
	if slot, ok := ci.InstanceOf.slotByName(name); ok && slot < len(ci.Fields) {
		ci.Fields[slot] = val
		return
	}
	ci.setDynamic(name, val)
}

func (ci *classInstance) setDynamic(name string, val Value) {
	if ci.Dynamic == nil {
		ci.Dynamic = make(map[string]Value)
	}
	ci.Dynamic[name] = val
}

// UpdateField replaces an existing field named by a literal with update(current)
func (ci *classInstance) UpdateField(f fields.ID, update func(Value) (Value, exception)) (exists bool, err exception) {
	if slot, ok := ci.InstanceOf.slot(f); ok && slot < len(ci.Fields) {
		return ci.updateSlot(slot, update)
	}
	return ci.updateDynamic(fields.Lookup(f), update)
}

// updateFieldByName replaces an existing field named by a runtime string with update(current)
func (ci *classInstance) updateFieldByName(name string, update func(Value) (Value, exception)) (exists bool, err exception) {
	if slot, ok := ci.InstanceOf.slotByName(name); ok && slot < len(ci.Fields) {
		return ci.updateSlot(slot, update)
	}
	return ci.updateDynamic(name, update)
}

func (ci *classInstance) updateSlot(slot int, update func(Value) (Value, exception)) (bool, exception) {
	value, exc := update(ci.Fields[slot])
	if exc == nil {
		ci.Fields[slot] = value
	}
	return true, exc
}

func (ci *classInstance) updateDynamic(name string, update func(Value) (Value, exception)) (bool, exception) {
	current, ok := ci.Dynamic[name]
	if !ok {
		return false, nil
	}
	value, exc := update(current)
	if exc == nil {
		ci.Dynamic[name] = value
	}
	return true, exc
}

// HasField reports whether a field named by a literal exists
func (ci *classInstance) HasField(f fields.ID) bool {
	if _, ok := ci.InstanceOf.slot(f); ok {
		return true
	}
	if ci.Dynamic != nil {
		_, ok := ci.Dynamic[fields.Lookup(f)]
		return ok
	}
	return false
}

// hasFieldByName reports whether a field named by a runtime string exists
func (ci *classInstance) hasFieldByName(name string) bool {
	if _, ok := ci.InstanceOf.slotByName(name); ok {
		return true
	}
	if ci.Dynamic != nil {
		_, ok := ci.Dynamic[name]
		return ok
	}
	return false
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

	iter := 0
	total := len(ci.Fields) + len(ci.Dynamic)

	writeField := func(name string, v Value) {
		builder.WriteString(name)
		builder.WriteString(": ")
		v.writeTo(builder, seen, true)
		if iter != total-1 {
			builder.WriteString(", ")
		}
		iter++
	}

	for i, fid := range ci.InstanceOf.Fields {
		if i >= len(ci.Fields) {
			break
		}
		writeField(fields.Lookup(fid), ci.Fields[i])
	}

	for _, name := range sortedKeys(ci.Dynamic) {
		writeField(name, ci.Dynamic[name])
	}

	builder.WriteByte('}')
}
