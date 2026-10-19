// Package math is Evie's math package: constants, rounding, powers and
// logarithms, trigonometry and random numbers.
package math

import (
	"errors"
	"fmt"
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
	pkg.Native("isFinite", vm.Func1(func(x float64) bool { return !math.IsInf(x, 0) && !math.IsNaN(x) }))
	pkg.Native("clamp", vm.Func3(func(x, lo, hi float64) float64 { return math.Min(math.Max(x, lo), hi) }))

	pkg.Variadic("min", 1, extreme(math.Min))
	pkg.Variadic("max", 1, extreme(math.Max))

	pkg.Native("random", vm.Func0(rand.Float64))
	pkg.Native("randomInt", vm.Func2E(func(lo, hi int) (int, error) {
		if hi < lo {
			return 0, errors.New("randomInt: the upper bound is below the lower bound")
		}
		return lo + rand.IntN(hi-lo+1), nil
	}))
	return pkg
}

// extreme folds its numeric arguments, or the numbers of a single array
// argument, with pick. It reads the arguments in place; Call.Args would copy
// them on every call.
func extreme(pick func(a, b float64) float64) vm.NativeFn {
	return func(c vm.Call) (vm.Value, error) {
		n, at := c.Len(), c.Arg
		if n == 1 {
			if arr, ok := c.Arg(0).AsArray(); ok {
				if len(arr.Data) == 0 {
					return vm.Value{}, errors.New("cannot take the extreme of an empty array")
				}
				n, at = len(arr.Data), func(i int) vm.Value { return arr.Data[i] }
			}
		}
		best := math.NaN()
		for i := 0; i < n; i++ {
			arg := at(i)
			x, ok := arg.AsFloat64()
			if !ok {
				return vm.Value{}, fmt.Errorf("expected numbers, got %s at position %d", arg.TypeOf(), i+1)
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
