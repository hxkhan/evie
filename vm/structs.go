package vm

import (
	"strings"
	"sync"

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
	MU         sync.RWMutex
	InstanceOf *UserStruct
	Fields     []Value             // Fixed array for declared fields: [0]=arg0, [1]=arg1
	Dynamic    map[fields.ID]Value // Lazily allocated ONLY if ad-hoc dynamic fields are added
}

func (usi *UserStructInstance) GetField(f fields.ID) (Value, bool) {
	usi.MU.RLock()
	defer usi.MU.RUnlock()

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
	usi.MU.Lock()
	defer usi.MU.Unlock()

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

func (usi *UserStructInstance) HasField(f fields.ID) bool {
	usi.MU.RLock()
	defer usi.MU.RUnlock()

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
	usi.MU.RLock()
	defer usi.MU.RUnlock()

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
