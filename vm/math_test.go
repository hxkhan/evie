package vm

import (
	"math"
	"testing"
)

func TestValueModBitExact(t *testing.T) {
	negZero := math.Copysign(0, -1)
	vals := []float64{
		0, negZero, 1, 2, 3, 7, 10, 1000,
		-1, -2, -7, -10,
		0.5, 1.5, 5.5, -5.5,
		float64(1<<53 - 1), float64(1 << 53), float64(1 << 54),
		math.Inf(1), math.Inf(-1), math.NaN(),
	}
	for _, a := range vals {
		for _, b := range vals {
			got, ok := BoxNumber(a).mod(BoxNumber(b))
			if !ok {
				t.Fatalf("mod(%v, %v): ok=false", a, b)
			}
			gotF, _ := got.AsFloat64()
			wantF := math.Mod(a, b)
			if math.IsNaN(wantF) {
				if !math.IsNaN(gotF) {
					t.Fatalf("mod(%v, %v) = %v, want NaN", a, b, gotF)
				}
				continue
			}
			if math.Float64bits(gotF) != math.Float64bits(wantF) {
				t.Fatalf("mod(%v, %v) = %v (0x%x), want %v (0x%x)",
					a, b, gotF, math.Float64bits(gotF), wantF, math.Float64bits(wantF))
			}
		}
	}
}
