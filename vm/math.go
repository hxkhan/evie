package vm

import "math"

func (x Value) Add(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxFloat64(math.Float64frombits(x.scalar) + math.Float64frombits(y.scalar)), true
	} else if x, ok := x.AsString(); ok {
		if y, ok := y.AsString(); ok {
			return BoxString(x + y), true
		}
	}
	return Value{}, false
}

func (x Value) Sub(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxFloat64(math.Float64frombits(x.scalar) - math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}

func (x Value) Mul(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxFloat64(math.Float64frombits(x.scalar) * math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}

func (x Value) Div(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxFloat64(math.Float64frombits(x.scalar) / math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}

func (x Value) Mod(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		x := math.Float64frombits(x.scalar)
		y := math.Float64frombits(y.scalar)
		return BoxFloat64(float64(int64(x) % int64(y))), true
	}
	return Value{}, false
}

func (x Value) LessThan(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxBool(math.Float64frombits(x.scalar) < math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}

func (x Value) GreaterThan(y Value) (z Value, ok bool) {
	if x.pointer == f64Type && y.pointer == f64Type {
		return BoxBool(math.Float64frombits(x.scalar) > math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}
