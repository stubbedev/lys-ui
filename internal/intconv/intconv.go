// Package intconv converts between integer types where Go's int meets
// the fixed-width integers of Wayland, D-Bus and the kernel: saturating
// where clamping is the meaning, checked where a value that does not fit
// is an error.
package intconv

import (
	"fmt"
	"math"
)

type integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

// Int32 is v saturated to the int32 range.
func Int32[T integer](v T) int32 {
	if v < 0 {
		return int32(min(max(int64(v), math.MinInt32), math.MaxInt32))
	}
	return int32(min(uint64(v), math.MaxInt32))
}

// Uint32 is v saturated to the uint32 range.
func Uint32[T integer](v T) uint32 {
	if v < 0 {
		return 0
	}
	return uint32(min(uint64(v), math.MaxUint32))
}

// Int32Bits is the int32 with the two's-complement bits of v: the
// reading wire formats give a signed 32-bit field.
func Int32Bits(v uint32) int32 {
	x := int32(v & math.MaxInt32)
	if v > math.MaxInt32 {
		x += math.MinInt32
	}
	return x
}

// CheckedInt32 is v as an int32, or an error when it does not fit.
func CheckedInt32[T integer](v T) (int32, error) {
	if v < 0 && int64(v) < math.MinInt32 || v >= 0 && uint64(v) > math.MaxInt32 {
		return 0, fmt.Errorf("intconv: %d overflows int32", v)
	}
	return Int32(v), nil
}

// CheckedUint32 is v as a uint32, or an error when it does not fit.
func CheckedUint32[T integer](v T) (uint32, error) {
	if v < 0 || uint64(v) > math.MaxUint32 {
		return 0, fmt.Errorf("intconv: %d overflows uint32", v)
	}
	return Uint32(v), nil
}
