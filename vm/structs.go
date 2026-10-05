package vm

import (
	"strings"

	"github.com/hxkhan/evie/vm/fields"
)

type userStruct struct {
	Name         string
	def          site
	Parent       *userStruct
	Fields       []fields.ID
	FieldOffsets map[fields.ID]int
	Methods      map[fields.ID]*userFn
}

// instantiate creates an instance from the constructor arguments
func (us *userStruct) instantiate(args []Value) (Value, exception) {
	if len(args) != len(us.Fields) {
		return Value{}, customError("struct '%v' takes %v argument(s) but %v were supplied", us.Name, len(us.Fields), len(args))
	}
	fields := make([]Value, len(args))
	copy(fields, args)
	return boxStructInstance(&structInstance{InstanceOf: us, Fields: fields}), nil
}

// construct evaluates the constructor arguments and creates an instance
func (us *userStruct) construct(fbr *fiber, site site, arguments []instruction) (Value, exception) {
	if len(arguments) != len(us.Fields) {
		return Value{}, errorAt(site, "struct '%v' takes %v argument(s) but %v were supplied", us.Name, len(us.Fields), len(arguments))
	}
	usi := &structInstance{InstanceOf: us, Fields: make([]Value, len(arguments))}
	for i, argument := range arguments {
		v, exc := argument(fbr)
		if exc != nil {
			return v, exc
		}
		usi.Fields[i] = v
	}
	return boxStructInstance(usi), nil
}

// lookupMethod finds a method on the struct or one of its ancestors
func (us *userStruct) lookupMethod(f fields.ID) *userFn {
	for cur := us; cur != nil; cur = cur.Parent {
		if fn, ok := cur.Methods[f]; ok {
			return fn
		}
	}
	return nil
}

// slot returns the index of a declared field, looked up by its interned id
func (us *userStruct) slot(f fields.ID) (int, bool) {
	slot, ok := us.FieldOffsets[f]
	return slot, ok
}

// slotByName returns the index of a declared field, looked up by name without
// interning it: a name that was never interned cannot be a declared field
func (us *userStruct) slotByName(name string) (int, bool) {
	if f, ok := fields.Find(name); ok {
		return us.slot(f)
	}
	return 0, false
}

type structInstance struct {
	InstanceOf *userStruct
	Fields     []Value          // Fixed array for declared fields: [0]=arg0, [1]=arg1
	Dynamic    map[string]Value // Lazily allocated ONLY if ad-hoc dynamic fields are added
}

// GetField reads a field named by a literal in the source
func (usi *structInstance) GetField(f fields.ID) (Value, bool) {
	if slot, ok := usi.InstanceOf.slot(f); ok && slot < len(usi.Fields) {
		return usi.Fields[slot], true
	}
	if usi.Dynamic != nil {
		val, exists := usi.Dynamic[fields.Lookup(f)]
		return val, exists
	}
	return Value{}, false
}

// getFieldByName reads a field named by a runtime string
func (usi *structInstance) getFieldByName(name string) (Value, bool) {
	if slot, ok := usi.InstanceOf.slotByName(name); ok && slot < len(usi.Fields) {
		return usi.Fields[slot], true
	}
	if usi.Dynamic != nil {
		val, exists := usi.Dynamic[name]
		return val, exists
	}
	return Value{}, false
}

// SetField writes a field named by a literal in the source, adding it if it is not declared
func (usi *structInstance) SetField(f fields.ID, val Value) {
	if slot, ok := usi.InstanceOf.slot(f); ok && slot < len(usi.Fields) {
		usi.Fields[slot] = val
		return
	}
	usi.setDynamic(fields.Lookup(f), val)
}

// setFieldByName writes a field named by a runtime string, adding it if it is not declared
func (usi *structInstance) setFieldByName(name string, val Value) {
	if slot, ok := usi.InstanceOf.slotByName(name); ok && slot < len(usi.Fields) {
		usi.Fields[slot] = val
		return
	}
	usi.setDynamic(name, val)
}

func (usi *structInstance) setDynamic(name string, val Value) {
	if usi.Dynamic == nil {
		usi.Dynamic = make(map[string]Value)
	}
	usi.Dynamic[name] = val
}

// UpdateField replaces an existing field named by a literal with update(current)
func (usi *structInstance) UpdateField(f fields.ID, update func(Value) (Value, exception)) (exists bool, err exception) {
	if slot, ok := usi.InstanceOf.slot(f); ok && slot < len(usi.Fields) {
		return usi.updateSlot(slot, update)
	}
	return usi.updateDynamic(fields.Lookup(f), update)
}

// updateFieldByName replaces an existing field named by a runtime string with update(current)
func (usi *structInstance) updateFieldByName(name string, update func(Value) (Value, exception)) (exists bool, err exception) {
	if slot, ok := usi.InstanceOf.slotByName(name); ok && slot < len(usi.Fields) {
		return usi.updateSlot(slot, update)
	}
	return usi.updateDynamic(name, update)
}

func (usi *structInstance) updateSlot(slot int, update func(Value) (Value, exception)) (bool, exception) {
	value, exc := update(usi.Fields[slot])
	if exc == nil {
		usi.Fields[slot] = value
	}
	return true, exc
}

func (usi *structInstance) updateDynamic(name string, update func(Value) (Value, exception)) (bool, exception) {
	current, ok := usi.Dynamic[name]
	if !ok {
		return false, nil
	}
	value, exc := update(current)
	if exc == nil {
		usi.Dynamic[name] = value
	}
	return true, exc
}

// HasField reports whether a field named by a literal exists
func (usi *structInstance) HasField(f fields.ID) bool {
	if _, ok := usi.InstanceOf.slot(f); ok {
		return true
	}
	if usi.Dynamic != nil {
		_, ok := usi.Dynamic[fields.Lookup(f)]
		return ok
	}
	return false
}

// hasFieldByName reports whether a field named by a runtime string exists
func (usi *structInstance) hasFieldByName(name string) bool {
	if _, ok := usi.InstanceOf.slotByName(name); ok {
		return true
	}
	if usi.Dynamic != nil {
		_, ok := usi.Dynamic[name]
		return ok
	}
	return false
}

func (usi *structInstance) String() string {
	builder := strings.Builder{}
	builder.WriteString(usi.InstanceOf.Name)
	builder.WriteByte('{')

	iter := 0
	total := len(usi.Fields) + len(usi.Dynamic)

	writeField := func(name string, v Value) {
		builder.WriteString(name)
		builder.WriteString(": ")
		if str, ok := v.AsString(); ok {
			builder.WriteByte('"')
			builder.WriteString(str)
			builder.WriteByte('"')
		} else {
			builder.WriteString(v.String())
		}
		if iter != total-1 {
			builder.WriteString(", ")
		}
		iter++
	}

	for i, fid := range usi.InstanceOf.Fields {
		if i >= len(usi.Fields) {
			break
		}
		writeField(fields.Lookup(fid), usi.Fields[i])
	}

	for _, name := range sortedKeys(usi.Dynamic) {
		writeField(name, usi.Dynamic[name])
	}

	builder.WriteByte('}')
	return builder.String()
}
