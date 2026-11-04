package vm

import (
	"strings"
	"unsafe"
)

// Array is the backing store of an evie array value
type Array struct {
	Data []Value
}

func (arr *Array) String() string {
	var builder strings.Builder
	arr.writeTo(&builder, nil)
	return builder.String()
}

func (arr *Array) writeTo(builder *strings.Builder, seen map[unsafe.Pointer]bool) {
	if arr == nil {
		builder.WriteString("[]")
		return
	}
	ptr := unsafe.Pointer(arr)
	if seen[ptr] {
		builder.WriteString("[...]")
		return
	}
	if seen == nil {
		seen = make(map[unsafe.Pointer]bool)
	}
	seen[ptr] = true
	defer delete(seen, ptr)

	builder.WriteByte('[')
	for i, v := range arr.Data {
		if i > 0 {
			builder.WriteString(", ")
		}
		v.writeTo(builder, seen, true)
	}
	builder.WriteByte(']')
}

func NewArray(values ...Value) *Array {
	return &Array{Data: values}
}
