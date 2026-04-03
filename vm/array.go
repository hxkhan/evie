package vm

import (
	"strings"
	"sync"
)

// Array is a thread-safe equivalent of go slices
type Array struct {
	MU   sync.RWMutex // temporarily exported
	Data []Value      // temporarily exported
}

func (arr *Array) String() string {
	builder := strings.Builder{}
	builder.WriteByte('[')

	arr.View(func(data []Value) {
		for i, v := range data {
			if str, ok := v.AsString(); ok {
				builder.WriteByte('"')
				builder.WriteString(str)
				builder.WriteByte('"')
			} else {
				builder.WriteString(v.String())
			}

			if i != len(data)-1 {
				builder.WriteString(", ")
			}
		}
	})

	builder.WriteByte(']')
	return builder.String()
}

func NewArray(values ...Value) *Array {
	return &Array{Data: values}
}

// View provides a safe and consistent snapshot like view for the duration of the viewers execution
func (arr *Array) View(viewer func(data []Value)) {
	arr.MU.RLock()
	defer arr.MU.RUnlock()
	viewer(arr.Data)
}
