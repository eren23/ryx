package stdlib

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/ryx-lang/ryx/pkg/vm"
)

// ---------------------------------------------------------------------------
// Math operations — work on Int and Float values
// ---------------------------------------------------------------------------

// Abs returns the absolute value of an Int or Float.
func Abs(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("abs: expected 1 argument, got %d", len(args))
	}
	switch args[0].Tag {
	case vm.TagInt:
		v := args[0].AsInt()
		if v < 0 {
			v = -v
		}
		return vm.IntVal(v), nil
	case vm.TagFloat:
		return vm.FloatVal(math.Abs(args[0].AsFloat())), nil
	default:
		return vm.UnitVal(), fmt.Errorf("abs: expected Int or Float, got tag %d", args[0].Tag)
	}
}

// Min returns the minimum of two Int or Float values.
func Min(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("min: expected 2 arguments, got %d", len(args))
	}
	if args[0].Tag == vm.TagInt && args[1].Tag == vm.TagInt {
		a, b := args[0].AsInt(), args[1].AsInt()
		if a < b {
			return vm.IntVal(a), nil
		}
		return vm.IntVal(b), nil
	}
	if args[0].Tag == vm.TagFloat && args[1].Tag == vm.TagFloat {
		return vm.FloatVal(math.Min(args[0].AsFloat(), args[1].AsFloat())), nil
	}
	// Mixed: promote Int to Float.
	a := toFloat(args[0])
	b := toFloat(args[1])
	return vm.FloatVal(math.Min(a, b)), nil
}

// Max returns the maximum of two Int or Float values.
func Max(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("max: expected 2 arguments, got %d", len(args))
	}
	if args[0].Tag == vm.TagInt && args[1].Tag == vm.TagInt {
		a, b := args[0].AsInt(), args[1].AsInt()
		if a > b {
			return vm.IntVal(a), nil
		}
		return vm.IntVal(b), nil
	}
	if args[0].Tag == vm.TagFloat && args[1].Tag == vm.TagFloat {
		return vm.FloatVal(math.Max(args[0].AsFloat(), args[1].AsFloat())), nil
	}
	a := toFloat(args[0])
	b := toFloat(args[1])
	return vm.FloatVal(math.Max(a, b)), nil
}

// Sqrt returns the square root of an Int or Float. Always returns Float.
func Sqrt(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("sqrt: expected 1 argument, got %d", len(args))
	}
	v := toFloat(args[0])
	return vm.FloatVal(math.Sqrt(v)), nil
}

// Pow returns base raised to exponent. Always returns Float.
func Pow(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("pow: expected 2 arguments, got %d", len(args))
	}
	base := toFloat(args[0])
	exp := toFloat(args[1])
	return vm.FloatVal(math.Pow(base, exp)), nil
}

// Floor returns the greatest integer value less than or equal to a Float.
func Floor(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("floor: expected 1 argument, got %d", len(args))
	}
	v := toFloat(args[0])
	return vm.FloatVal(math.Floor(v)), nil
}

// Ceil returns the least integer value greater than or equal to a Float.
func Ceil(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("ceil: expected 1 argument, got %d", len(args))
	}
	v := toFloat(args[0])
	return vm.FloatVal(math.Ceil(v)), nil
}

// Round returns the nearest integer, rounding half away from zero.
func Round(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("round: expected 1 argument, got %d", len(args))
	}
	v := toFloat(args[0])
	return vm.FloatVal(math.Round(v)), nil
}

// Sin returns the sine of an Int or Float in radians. Always returns Float.
func Sin(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("sin: expected 1 arg, got %d", len(args))
	}
	return vm.FloatVal(math.Sin(toFloat(args[0]))), nil
}

// Cos returns the cosine of an Int or Float in radians. Always returns Float.
func Cos(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("cos: expected 1 arg, got %d", len(args))
	}
	return vm.FloatVal(math.Cos(toFloat(args[0]))), nil
}

// Tan returns the tangent of an Int or Float in radians. Always returns Float.
func Tan(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("tan: expected 1 arg, got %d", len(args))
	}
	return vm.FloatVal(math.Tan(toFloat(args[0]))), nil
}

// Asin returns the arcsine of an Int or Float. Always returns Float.
func Asin(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("asin: expected 1 arg, got %d", len(args))
	}
	return vm.FloatVal(math.Asin(toFloat(args[0]))), nil
}

// Acos returns the arccosine of an Int or Float. Always returns Float.
func Acos(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("acos: expected 1 arg, got %d", len(args))
	}
	return vm.FloatVal(math.Acos(toFloat(args[0]))), nil
}

// Atan returns the arctangent of an Int or Float. Always returns Float.
func Atan(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("atan: expected 1 arg, got %d", len(args))
	}
	return vm.FloatVal(math.Atan(toFloat(args[0]))), nil
}

// Atan2 returns the arctangent of y/x using the signs of both arguments.
func Atan2(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("atan2: expected 2 args, got %d", len(args))
	}
	return vm.FloatVal(math.Atan2(toFloat(args[0]), toFloat(args[1]))), nil
}

// Log returns the natural logarithm of an Int or Float. Always returns Float.
func Log(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("log: expected 1 arg, got %d", len(args))
	}
	return vm.FloatVal(math.Log(toFloat(args[0]))), nil
}

// Log2 returns the base-2 logarithm of an Int or Float. Always returns Float.
func Log2(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("log2: expected 1 arg, got %d", len(args))
	}
	return vm.FloatVal(math.Log2(toFloat(args[0]))), nil
}

// Log10 returns the base-10 logarithm of an Int or Float. Always returns Float.
func Log10(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("log10: expected 1 arg, got %d", len(args))
	}
	return vm.FloatVal(math.Log10(toFloat(args[0]))), nil
}

// Exp returns e raised to the power of an Int or Float. Always returns Float.
func Exp(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 1 {
		return vm.UnitVal(), fmt.Errorf("exp: expected 1 arg, got %d", len(args))
	}
	return vm.FloatVal(math.Exp(toFloat(args[0]))), nil
}

// Pi returns the constant pi.
func Pi(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("pi: expected 0 args, got %d", len(args))
	}
	return vm.FloatVal(math.Pi), nil
}

// E returns Euler's number.
func E(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("e: expected 0 args, got %d", len(args))
	}
	return vm.FloatVal(math.E), nil
}

// Gcd returns the greatest common divisor of two Int values.
func Gcd(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("gcd: expected 2 args, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt || args[1].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("gcd: both arguments must be Int")
	}
	a := absInt(args[0].AsInt())
	b := absInt(args[1].AsInt())
	for b != 0 {
		a, b = b, a%b
	}
	return vm.IntVal(a), nil
}

// Lcm returns the least common multiple of two Int values.
func Lcm(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("lcm: expected 2 args, got %d", len(args))
	}
	if args[0].Tag != vm.TagInt || args[1].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("lcm: both arguments must be Int")
	}
	a := absInt(args[0].AsInt())
	b := absInt(args[1].AsInt())
	if a == 0 || b == 0 {
		return vm.IntVal(0), nil
	}
	ga, gb := a, b
	for gb != 0 {
		ga, gb = gb, ga%gb
	}
	return vm.IntVal(a / ga * b), nil
}

// Clamp bounds a value to [lo, hi], preserving Int results when all args are Int.
func Clamp(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 3 {
		return vm.UnitVal(), fmt.Errorf("clamp: expected 3 args, got %d", len(args))
	}
	if args[0].Tag == vm.TagInt && args[1].Tag == vm.TagInt && args[2].Tag == vm.TagInt {
		x := args[0].AsInt()
		lo := args[1].AsInt()
		hi := args[2].AsInt()
		if x < lo {
			return vm.IntVal(lo), nil
		}
		if x > hi {
			return vm.IntVal(hi), nil
		}
		return vm.IntVal(x), nil
	}
	x := toFloat(args[0])
	lo := toFloat(args[1])
	hi := toFloat(args[2])
	if x < lo {
		return vm.FloatVal(lo), nil
	}
	if x > hi {
		return vm.FloatVal(hi), nil
	}
	return vm.FloatVal(x), nil
}

// RandomInt returns a random integer in [low, high).
func RandomInt(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 2 {
		return vm.UnitVal(), fmt.Errorf("random_int: expected 2 arguments (low, high), got %d", len(args))
	}
	if args[0].Tag != vm.TagInt || args[1].Tag != vm.TagInt {
		return vm.UnitVal(), fmt.Errorf("random_int: expected Int arguments")
	}
	low := args[0].AsInt()
	high := args[1].AsInt()
	if low >= high {
		return vm.UnitVal(), fmt.Errorf("random_int: low (%d) must be less than high (%d)", low, high)
	}
	n := rand.Int64N(high-low) + low
	return vm.IntVal(n), nil
}

// RandomFloat returns a random float in [0.0, 1.0).
func RandomFloat(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
	if len(args) != 0 {
		return vm.UnitVal(), fmt.Errorf("random_float: expected 0 arguments, got %d", len(args))
	}
	return vm.FloatVal(rand.Float64()), nil
}

// toFloat converts an Int or Float value to float64.
func toFloat(v vm.Value) float64 {
	switch v.Tag {
	case vm.TagInt:
		return float64(v.AsInt())
	case vm.TagFloat:
		return v.AsFloat()
	default:
		return 0
	}
}

func absInt(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
