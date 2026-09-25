package vm

import (
	"strings"

	"github.com/hxkhan/evie/vm/fields"
)

type UserStruct struct {
	Name         string
	Parent       *UserStruct
	Fields       []fields.ID
	FieldOffsets map[fields.ID]int
	Methods      map[fields.ID]*UserFn
}

type UserStructInstance struct {
	InstanceOf *UserStruct
	Fields     []Value             // Fixed array for declared fields: [0]=arg0, [1]=arg1
	Dynamic    map[fields.ID]Value // Lazily allocated ONLY if ad-hoc dynamic fields are added
}

func (usi *UserStructInstance) GetField(f fields.ID) (Value, bool) {
	if slot, ok := usi.InstanceOf.FieldOffsets[f]; ok {
		if slot < len(usi.Fields) {
			return usi.Fields[slot], true
		}
	}
	if usi.Dynamic != nil {
		val, exists := usi.Dynamic[f]
		return val, exists
	}
	return Value{}, false
}

func (usi *UserStructInstance) SetField(f fields.ID, val Value) {
	if slot, ok := usi.InstanceOf.FieldOffsets[f]; ok {
		if slot < len(usi.Fields) {
			usi.Fields[slot] = val
			return
		}
	}
	if usi.Dynamic == nil {
		usi.Dynamic = make(map[fields.ID]Value)
	}
	usi.Dynamic[f] = val
}

// UpdateField replaces an existing field with update(current)
func (usi *UserStructInstance) UpdateField(f fields.ID, update func(Value) (Value, Exception)) (exists bool, err Exception) {
	if slot, ok := usi.InstanceOf.FieldOffsets[f]; ok && slot < len(usi.Fields) {
		value, exc := update(usi.Fields[slot])
		if exc == nil {
			usi.Fields[slot] = value
		}
		return true, exc
	}
	if current, ok := usi.Dynamic[f]; ok {
		value, exc := update(current)
		if exc == nil {
			usi.Dynamic[f] = value
		}
		return true, exc
	}
	return false, nil
}

func (usi *UserStructInstance) HasField(f fields.ID) bool {
	if _, ok := usi.InstanceOf.FieldOffsets[f]; ok {
		return true
	}
	if usi.Dynamic != nil {
		_, ok := usi.Dynamic[f]
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

	for i, fid := range usi.InstanceOf.Fields {
		if i >= len(usi.Fields) {
			break
		}
		builder.WriteString(fields.Lookup(fid))
		builder.WriteString(": ")

		v := usi.Fields[i]
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

	if len(usi.Dynamic) > 0 {
		for f, v := range usi.Dynamic {
			builder.WriteString(fields.Lookup(f))
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
	}

	builder.WriteByte('}')
	return builder.String()
}
