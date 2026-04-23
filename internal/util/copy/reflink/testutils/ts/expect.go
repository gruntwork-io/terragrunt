package ts

import (
	"errors"
	"reflect"
	"syscall"
	"testing"
)

func NoErr[T any](ret T, err error) func(t testing.TB) T {
	return func(tb testing.TB) T {
		tb.Helper()

		if err != nil {
			tb.Fatalf("unexpected error: %v", err)
		}

		return ret
	}
}

func True(ret bool) func(t testing.TB) bool {
	return func(tb testing.TB) bool {
		tb.Helper()

		if !ret {
			tb.Fatalf("expected true, got false")
		}

		return ret
	}
}

func Is[T any](val T) func(tb testing.TB, actual T) T {
	return func(tb testing.TB, actual T) T {
		tb.Helper()

		if !reflect.DeepEqual(actual, val) {
			tb.Fatalf("expected %v, got %v", val, actual)
		}

		return val
	}
}

func IsErr(tb testing.TB, err error, target error) {
	tb.Helper()

	if !errors.Is(err, target) {
		var err syscall.Errno
		if errors.As(err, &err) {
			tb.Logf("hint: received syscall error %v", uintptr(err))
		}

		tb.Fatalf("expected error %q, got %q", target, err)
	}
}
