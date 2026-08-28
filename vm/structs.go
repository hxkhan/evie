package vm

import (
	"strings"
	"sync"

	"github.com/hxkhan/evie/vm/fields"
)

type UserStruct struct {
	Name    string
	Parent  *UserStruct
	Fields  []fields.ID
	Methods map[fields.ID]*UserFn
}

type UserStructInstance struct {
	MU         sync.RWMutex
	InstanceOf *UserStruct
	Fields     map[fields.ID]Value
}

func (usi *UserStructInstance) String() string {
	usi.MU.RLock()
	defer usi.MU.RUnlock()

	builder := strings.Builder{}
	builder.WriteString(usi.InstanceOf.Name)
	builder.WriteByte('{')

	iter := 0
	for i, v := range usi.Fields {
		builder.WriteString(fields.Lookup(i))
		builder.WriteByte(':')
		builder.WriteByte(' ')

		if str, ok := v.AsString(); ok {
			builder.WriteByte('"')
			builder.WriteString(str)
			builder.WriteByte('"')
		} else {
			builder.WriteString(v.String())
		}

		if iter != len(usi.Fields)-1 {
			builder.WriteString(", ")
		}

		iter += 1
	}

	builder.WriteByte('}')
	return builder.String()
}
