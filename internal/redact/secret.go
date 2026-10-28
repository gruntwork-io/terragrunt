package redact

// Secret is a string that must not appear in logs or errors. Formatting one
// renders [Placeholder]. Only [Secret.Reveal] returns the string.
//
// fmt does not call String on unexported struct fields, so formatting a struct
// with %v prints the raw string of a Secret in an unexported field.
type Secret struct {
	raw string
}

// NewSecret wraps raw.
func NewSecret(raw string) Secret {
	return Secret{raw: raw}
}

// String renders [Placeholder].
func (s Secret) String() string {
	return Placeholder
}

// GoString renders [Placeholder]. fmt uses it for %#v, which ignores String.
func (s Secret) GoString() string {
	return Placeholder
}

// Reveal returns the wrapped string.
func (s Secret) Reveal() string {
	return s.raw
}
