// Package math is Evie's math package: constants, rounding, powers and
// logarithms, trigonometry and random numbers.
package math

import (
	"math"
	"math/rand/v2"

	"github.com/hxkhan/evie/vm"
)

// Construct builds the math package
func Construct() *vm.Package {
	pkg := vm.NewPackage("math")

	pkg.Set("pi", vm.BoxNumber(math.Pi))
	pkg.Set("e", vm.BoxNumber(math.E))
	pkg.Set("inf", vm.BoxNumber(math.Inf(1)))
	pkg.Set("nan", vm.BoxNumber(math.NaN()))
	pkg.Set("maxSafeInt", vm.BoxNumber(1<<53-1))

	unary := map[string]func(float64) float64{
		"abs":   math.Abs,
		"floor": math.Floor,
		"ceil":  math.Ceil,
		"round": math.Round,
		"trunc": math.Trunc,
		"sqrt":  math.Sqrt,
		"cbrt":  math.Cbrt,
		"exp":   math.Exp,
		"log":   math.Log,
		"log2":  math.Log2,
		"log10": math.Log10,
		"sin":   math.Sin,
		"cos":   math.Cos,
		"tan":   math.Tan,
		"asin":  math.Asin,
		"acos":  math.Acos,
		"atan":  math.Atan,
		"sinh":  math.Sinh,
		"cosh":  math.Cosh,
		"tanh":  math.Tanh,
		"sign": func(x float64) float64 {
			switch {
			case x > 0:
				return 1
			case x < 0:
				return -1
			}
			return x // keeps 0, -0 and NaN
		},
	}
	for name, fn := range unary {
		pkg.Native(name, vm.Func1(fn))
	}

	pkg.Native("pow", vm.Func2(math.Pow))
	pkg.Native("atan2", vm.Func2(math.Atan2))
	pkg.Native("hypot", vm.Func2(math.Hypot))
	pkg.Native("isNaN", vm.Func1(math.IsNaN))
	pkg.Native("isInf", vm.Func1(func(x float64) bool { return math.IsInf(x, 0) }))
	pkg.Native("isFinite", vm.Func1(func(x float64) bool { return !math.IsInf(x, 0) && !math.IsNaN(x) }))
	pkg.Native("clamp", vm.Func3E(func(x, lo, hi float64) (float64, error) {
		if lo > hi {
			return 0, vm.Errorf(vm.KindValue, "clamp: the lower bound exceeds the upper bound")
		}
		return math.Min(math.Max(x, lo), hi), nil
	}))

	pkg.Variadic("min", 1, extreme("min", math.Min))
	pkg.Variadic("max", 1, extreme("max", math.Max))

	pkg.Native("random", vm.Func0(rand.Float64))
	pkg.Native("randomInt", vm.Func2E(func(lo, hi int) (int, error) {
		if hi < lo {
			return 0, vm.Errorf(vm.KindValue, "randomInt: the upper bound is below the lower bound")
		}
		span := uint64(hi) - uint64(lo)
		if span == math.MaxUint64 {
			return int(rand.Uint64()), nil
		}
		return int(uint64(lo) + rand.Uint64N(span+1)), nil
	}))
	// what tools such as editors show for each member
	pkg.Document("pi", "pi", "The ratio of a circle's circumference to its diameter.")
	pkg.Document("e", "e", "Euler's number, the base of natural logarithms.")
	pkg.Document("inf", "inf", "Positive infinity.")
	pkg.Document("nan", "nan", "Not a number.")
	pkg.Document("maxSafeInt", "maxSafeInt", "The largest integer a number holds exactly, 2^53 - 1.")
	pkg.Document("abs", "abs(x)", "The absolute value of x.")
	pkg.Document("sign", "sign(x)", "-1, 0 or 1, by the sign of x.")
	pkg.Document("floor", "floor(x)", "x rounded down.")
	pkg.Document("ceil", "ceil(x)", "x rounded up.")
	pkg.Document("round", "round(x)", "x rounded to the nearest integer, halves away from zero.")
	pkg.Document("trunc", "trunc(x)", "x without its fraction.")
	pkg.Document("sqrt", "sqrt(x)", "The square root of x.")
	pkg.Document("cbrt", "cbrt(x)", "The cube root of x.")
	pkg.Document("pow", "pow(x, y)", "x to the power y.")
	pkg.Document("exp", "exp(x)", "e to the power x.")
	pkg.Document("log", "log(x)", "The natural logarithm of x.")
	pkg.Document("log2", "log2(x)", "The base-2 logarithm of x.")
	pkg.Document("log10", "log10(x)", "The base-10 logarithm of x.")
	pkg.Document("hypot", "hypot(x, y)", "The length of the hypotenuse, sqrt(x*x + y*y).")
	pkg.Document("sin", "sin(x)", "The sine of x, an angle in radians.")
	pkg.Document("cos", "cos(x)", "The cosine of x, an angle in radians.")
	pkg.Document("tan", "tan(x)", "The tangent of x, an angle in radians.")
	pkg.Document("asin", "asin(x)", "The arcsine of x, in radians.")
	pkg.Document("acos", "acos(x)", "The arccosine of x, in radians.")
	pkg.Document("atan", "atan(x)", "The arctangent of x, in radians.")
	pkg.Document("atan2", "atan2(y, x)", "The angle of the point (x, y) from the x axis, in radians.")
	pkg.Document("sinh", "sinh(x)", "The hyperbolic sine of x.")
	pkg.Document("cosh", "cosh(x)", "The hyperbolic cosine of x.")
	pkg.Document("tanh", "tanh(x)", "The hyperbolic tangent of x.")
	pkg.Document("min", "min(...values)", "The smallest of the arguments, or of one array.")
	pkg.Document("max", "max(...values)", "The largest of the arguments, or of one array.")
	pkg.Document("clamp", "clamp(x, lo, hi)", "x limited to the range lo to hi.")
	pkg.Document("isNaN", "isNaN(x)", "Whether x is not a number.")
	pkg.Document("isInf", "isInf(x)", "Whether x is infinite.")
	pkg.Document("isFinite", "isFinite(x)", "Whether x is neither infinite nor not a number.")
	pkg.Document("random", "random()", "A random number in [0, 1).")
	pkg.Document("randomInt", "randomInt(lo, hi)", "A random integer in [lo, hi].")
	return pkg
}

// extreme folds its numeric arguments, or the numbers of a single array
// argument, with pick. It reads the arguments in place; Call.Args would copy
// them on every call.
func extreme(name string, pick func(a, b float64) float64) vm.NativeFn {
	return func(c vm.Call) (vm.Value, error) {
		n, at := c.Len(), c.Arg
		if n == 1 {
			if arr, ok := c.Arg(0).AsArray(); ok {
				if len(arr.Data) == 0 {
					return vm.Value{}, vm.Errorf(vm.KindValue, "%s: cannot take %s of an empty array", name, name)
				}
				n, at = len(arr.Data), func(i int) vm.Value { return arr.Data[i] }
			}
		}
		best := math.NaN()
		for i := 0; i < n; i++ {
			arg := at(i)
			x, ok := arg.AsFloat64()
			if !ok {
				return vm.Value{}, vm.Errorf(vm.KindType, "%s: expected numbers, got %s at position %d", name, arg.TypeOf(), i+1)
			}
			if i == 0 {
				best = x
			} else {
				best = pick(best, x)
			}
		}
		return vm.BoxNumber(best), nil
	}
}
