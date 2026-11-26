package vm

import (
	"strings"
	"unsafe"
)

// Array is the backing store of an evie array value
type Array struct {
	Data []Value
}

// String returns the array as Evie prints it, such as [1, "a"]
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

// push appends v. A full array doubles its capacity: append grows a large
// slice by only a quarter, which copies a growing array far more often.
func (arr *Array) push(v Value) {
	if len(arr.Data) == cap(arr.Data) {
		arr.grow()
	}
	arr.Data = append(arr.Data, v)
}

//go:noinline
func (arr *Array) grow() {
	grown := make([]Value, len(arr.Data), max(2*cap(arr.Data), 4))
	copy(grown, arr.Data)
	arr.Data = grown
}

// NewArray makes an array holding values, without copying them
func NewArray(values ...Value) *Array {
	return &Array{Data: values}
}
