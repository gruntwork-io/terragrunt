package util

// Deref returns the pointed-to value, or the zero value of T when nil. Use it
// for tri-state settings, where a nil pointer means the user did not say.
func Deref[T any](v *T) T {
	if v == nil {
		var zero T

		return zero
	}

	return *v
}
