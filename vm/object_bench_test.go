package vm

import (
	"fmt"
	"testing"
)

// the objects benchmark of the cross-language suite, on the table alone:
// 500 keys set, updated and read
func BenchmarkObject500(b *testing.B) {
	keys := make([]string, 500)
	for i := range keys {
		keys[i] = fmt.Sprintf("k%d", i)
	}
	for b.Loop() {
		o := NewObject(0)
		for j, k := range keys {
			o.Set(k, BoxNumber(float64(j)))
		}
		for _, k := range keys {
			v, _ := o.Get(k)
			o.Set(k, v)
		}
		for _, k := range keys {
			o.Get(k)
		}
	}
}
