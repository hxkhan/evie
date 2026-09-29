package vm

import (
	"strings"

	"github.com/hxkhan/evie/vm/fields"
)

type UserStruct struct {
	Name         string
	def          site
	Parent       *UserStruct
	Fields       []fields.ID
	FieldOffsets map[fields.ID]int
	Methods      map[fields.ID]*UserFn
}

// instantiate creates an instance from the constructor arguments
func (us *UserStruct) instantiate(args []Value) (Value, Exception) {
	if len(args) != len(us.Fields) {
		return Value{}, CustomError("struct '%v' takes %v argument(s) but %v were supplied", us.Name, len(us.Fields), len(args))
	}
	fields := make([]Value, len(args))
	copy(fields, args)
	return BoxUserStructInstance(&UserStructInstance{InstanceOf: us, Fields: fields}), nil
}

// construct evaluates the constructor arguments and creates an instance
func (us *UserStruct) construct(fbr *Fiber, site site, arguments []instruction) (Value, Exception) {
	if len(arguments) != len(us.Fields) {
		return Value{}, errorAt(site, "struct '%v' takes %v argument(s) but %v were supplied", us.Name, len(us.Fields), len(arguments))
	}
	usi := &UserStructInstance{InstanceOf: us, Fields: make([]Value, len(arguments))}
	for i, argument := range arguments {
		v, exc := argument(fbr)
		if exc != nil {
			return v, exc
		}
		usi.Fields[i] = v
	}
	return BoxUserStructInstance(usi), nil
}

// lookupMethod finds a method on the struct or one of its ancestors
func (us *UserStruct) lookupMethod(f fields.ID) *UserFn {
	for cur := us; cur != nil; cur = cur.Parent {
		if fn, ok := cur.Methods[f]; ok {
			return fn
		}
	}
	return nil
}

// slot returns the index of a declared field, looked up by its interned id
func (us *UserStruct) slot(f fields.ID) (int, bool) {
	slot, ok := us.FieldOffsets[f]
	return slot, ok
}

// slotByName returns the index of a declared field, looked up by name without
// interning it: a name that was never interned cannot be a declared field
func (us *UserStruct) slotByName(name string) (int, bool) {
	if f, ok := fields.Find(name); ok {
		return us.slot(f)
	}
	return 0, false
}

type UserStructInstance struct {
	InstanceOf *UserStruct
	Fields     []Value          // Fixed array for declared fields: [0]=arg0, [1]=arg1
	Dynamic    map[string]Value // Lazily allocated ONLY if ad-hoc dynamic fields are added
}

// GetField reads a field named by a literal in the source
func (usi *UserStructInstance) GetField(f fields.ID) (Value, bool) {
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
func (usi *UserStructInstance) getFieldByName(name string) (Value, bool) {
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
func (usi *UserStructInstance) SetField(f fields.ID, val Value) {
	if slot, ok := usi.InstanceOf.slot(f); ok && slot < len(usi.Fields) {
		usi.Fields[slot] = val
		return
	}
	usi.setDynamic(fields.Lookup(f), val)
}

// setFieldByName writes a field named by a runtime string, adding it if it is not declared
func (usi *UserStructInstance) setFieldByName(name string, val Value) {
	if slot, ok := usi.InstanceOf.slotByName(name); ok && slot < len(usi.Fields) {
		usi.Fields[slot] = val
		return
	}
	usi.setDynamic(name, val)
}

func (usi *UserStructInstance) setDynamic(name string, val Value) {
	if usi.Dynamic == nil {
		usi.Dynamic = make(map[string]Value)
	}
	usi.Dynamic[name] = val
}

// UpdateField replaces an existing field named by a literal with update(current)
func (usi *UserStructInstance) UpdateField(f fields.ID, update func(Value) (Value, Exception)) (exists bool, err Exception) {
	if slot, ok := usi.InstanceOf.slot(f); ok && slot < len(usi.Fields) {
		return usi.updateSlot(slot, update)
	}
	return usi.updateDynamic(fields.Lookup(f), update)
}

// updateFieldByName replaces an existing field named by a runtime string with update(current)
func (usi *UserStructInstance) updateFieldByName(name string, update func(Value) (Value, Exception)) (exists bool, err Exception) {
	if slot, ok := usi.InstanceOf.slotByName(name); ok && slot < len(usi.Fields) {
		return usi.updateSlot(slot, update)
	}
	return usi.updateDynamic(name, update)
}

func (usi *UserStructInstance) updateSlot(slot int, update func(Value) (Value, Exception)) (bool, Exception) {
	value, exc := update(usi.Fields[slot])
	if exc == nil {
		usi.Fields[slot] = value
	}
	return true, exc
}

func (usi *UserStructInstance) updateDynamic(name string, update func(Value) (Value, Exception)) (bool, Exception) {
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
func (usi *UserStructInstance) HasField(f fields.ID) bool {
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
func (usi *UserStructInstance) hasFieldByName(name string) bool {
	if _, ok := usi.InstanceOf.slotByName(name); ok {
		return true
	}
	if usi.Dynamic != nil {
		_, ok := usi.Dynamic[name]
		return ok
	}
	return false
}

func (usi *UserStructInstance) String() string {
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
