package vm

import (
	"math"

	"github.com/hxkhan/evie/ast"
)

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
		return modNumbers(math.Float64frombits(x.scalar), math.Float64frombits(y.scalar)), true
	}
	return Value{}, false
}

// modNumbers is % on two numbers
func modNumbers(xf, yf float64) Value {
	// Fast path for exact non-negative integers in [0, 2^53): hardware int64 %
	// is bit-for-bit identical to IEEE-754 math.Mod and ~30-40x faster.
	if !math.Signbit(xf) && xf < 1<<53 && yf > 0 && yf < 1<<53 { // the sign bit keeps -0 % y at -0

		ix, iy := int64(xf), int64(yf)
		if float64(ix) == xf && float64(iy) == yf {
			return Value{scalar: math.Float64bits(float64(ix % iy)), pointer: f64Type}
		}
	}
	return slowMod(xf, yf)
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

// fastCompound is the number path of a compound assignment such as `x.n += 1`:
// it applies op to x and y when both are numbers and op is + - * or /, and is
// small enough to inline at every site; anything else takes the general path
func fastCompound(op ast.Operator, x, y Value) (Value, bool) {
	if x.pointer != f64Type || y.pointer != f64Type || op > ast.DivOp {
		return Value{}, false
	}
	return BoxNumber(arith(op, math.Float64frombits(x.scalar), math.Float64frombits(y.scalar))), true
}

// arith applies + - * or / to two numbers
func arith(op ast.Operator, a, b float64) float64 {
	switch op {
	case ast.AddOp:
		return a + b
	case ast.SubOp:
		return a - b
	case ast.MulOp:
		return a * b
	}
	return a / b
}
