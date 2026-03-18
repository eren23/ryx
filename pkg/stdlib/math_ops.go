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
