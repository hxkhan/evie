package vm

import "strings"

// Array is the backing store of an evie array value
type Array struct {
	Data []Value
}

func (arr *Array) String() string {
	builder := strings.Builder{}
	builder.WriteByte('[')

	for i, v := range arr.Data {
		if str, ok := v.AsString(); ok {
			builder.WriteByte('"')
			builder.WriteString(str)
			builder.WriteByte('"')
		} else {
			builder.WriteString(v.String())
		}

		if i != len(arr.Data)-1 {
			builder.WriteString(", ")
		}
	}

	builder.WriteByte(']')
	return builder.String()
}

func NewArray(values ...Value) *Array {
	return &Array{Data: values}
}
