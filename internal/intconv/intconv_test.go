package intconv

import (
	"math"
	"testing"
)

func TestInt32Saturates(t *testing.T) {
	cases := []struct {
		in   int64
		want int32
	}{
		{0, 0},
		{-7, -7},
		{math.MaxInt32, math.MaxInt32},
		{math.MaxInt32 + 1, math.MaxInt32},
		{math.MinInt32, math.MinInt32},
		{math.MinInt32 - 1, math.MinInt32},
		{math.MaxInt64, math.MaxInt32},
		{math.MinInt64, math.MinInt32},
	}
	for _, c := range cases {
		if got := Int32(c.in); got != c.want {
			t.Errorf("Int32(%d) = %d, want %d", c.in, got, c.want)
		}
	}
	if got := Int32(uint32(math.MaxUint32)); got != math.MaxInt32 {
		t.Errorf("Int32(MaxUint32) = %d, want MaxInt32", got)
	}
	if got := Int32(uint64(math.MaxUint64)); got != math.MaxInt32 {
		t.Errorf("Int32(MaxUint64) = %d, want MaxInt32", got)
	}
}

func TestUint32Saturates(t *testing.T) {
	cases := []struct {
		in   int64
		want uint32
	}{
		{0, 0},
		{-1, 0},
		{math.MinInt64, 0},
		{42, 42},
		{math.MaxUint32, math.MaxUint32},
		{math.MaxUint32 + 1, math.MaxUint32},
	}
	for _, c := range cases {
		if got := Uint32(c.in); got != c.want {
			t.Errorf("Uint32(%d) = %d, want %d", c.in, got, c.want)
		}
	}
	if got := Uint32(uint64(math.MaxUint64)); got != math.MaxUint32 {
		t.Errorf("Uint32(MaxUint64) = %d, want MaxUint32", got)
	}
}

func TestCheckedInt32(t *testing.T) {
	for _, v := range []int64{0, -1, math.MaxInt32, math.MinInt32} {
		got, err := CheckedInt32(v)
		if err != nil || int64(got) != v {
			t.Errorf("CheckedInt32(%d) = %d, %v; want %d, nil", v, got, err, v)
		}
	}
	for _, v := range []int64{math.MaxInt32 + 1, math.MinInt32 - 1, math.MaxInt64} {
		if got, err := CheckedInt32(v); err == nil {
			t.Errorf("CheckedInt32(%d) = %d, nil; want an error", v, got)
		}
	}
	if _, err := CheckedInt32(uintptr(math.MaxUint32)); err == nil {
		t.Error("CheckedInt32(uintptr(MaxUint32)) succeeded; want an error")
	}
}

func TestCheckedUint32(t *testing.T) {
	for _, v := range []int64{0, 1, math.MaxUint32} {
		got, err := CheckedUint32(v)
		if err != nil || int64(got) != v {
			t.Errorf("CheckedUint32(%d) = %d, %v; want %d, nil", v, got, err, v)
		}
	}
	for _, v := range []int64{-1, math.MaxUint32 + 1, math.MinInt64} {
		if got, err := CheckedUint32(v); err == nil {
			t.Errorf("CheckedUint32(%d) = %d, nil; want an error", v, got)
		}
	}
}

func TestInt32Bits(t *testing.T) {
	cases := []struct {
		in   uint32
		want int32
	}{
		{0, 0},
		{1, 1},
		{math.MaxInt32, math.MaxInt32},
		{math.MaxInt32 + 1, math.MinInt32},
		{math.MaxUint32, -1},
		{0xfffffffe, -2},
	}
	for _, c := range cases {
		if got := Int32Bits(c.in); got != c.want {
			t.Errorf("Int32Bits(%#x) = %d, want %d", c.in, got, c.want)
		}
	}
}
