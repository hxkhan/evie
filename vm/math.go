package vm

import "math"

func (x Value) add(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxNumber(math.Float64frombits(x.scalar) + math.Float64frombits(y.scalar)), true
	} else if x, ok := x.AsString(); ok {
		if y, ok := y.AsString(); ok {
			return BoxString(x + y), true
		}
	}
	return Value{}, false
}

func (x Value) sub(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxNumber(math.Float64frombits(x.scalar) - math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}

func (x Value) mul(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxNumber(math.Float64frombits(x.scalar) * math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}

func (x Value) div(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxNumber(math.Float64frombits(x.scalar) / math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}

//go:noinline
func slowMod(x, y float64) Value {
	return BoxNumber(math.Mod(x, y))
}

func (x Value) mod(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		xf := math.Float64frombits(x.scalar)
		yf := math.Float64frombits(y.scalar)
		// Fast path for exact non-negative integers in [0, 2^53): hardware int64 %
		// is bit-for-bit identical to IEEE-754 math.Mod and ~30-40x faster.
		if x.scalar&(1<<63) == 0 && xf < 1<<53 && yf > 0 && yf < 1<<53 {
			ix, iy := int64(xf), int64(yf)
			if float64(ix) == xf && float64(iy) == yf {
				return Value{scalar: math.Float64bits(float64(ix % iy)), pointer: f64Type}, true
			}
		}
		return slowMod(xf, yf), true
	}
	return Value{}, false
}

func (x Value) lessThan(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxBool(math.Float64frombits(x.scalar) < math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}

func (x Value) greaterThan(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxBool(math.Float64frombits(x.scalar) > math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}

func (x Value) lessThanOrEqualTo(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxBool(math.Float64frombits(x.scalar) <= math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}

func (x Value) greaterThanOrEqualTo(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxBool(math.Float64frombits(x.scalar) >= math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}
