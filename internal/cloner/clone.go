// Package cloner provides functions to deep clone any Go data.
package cloner

// Clone recursively deep clones src into a new value on the heap.
func Clone[T any](src *T, opts ...Option) *T {
	conf := &Config{}
	for _, opt := range opts {
		opt(conf)
	}

	cloner := Cloner[T]{Config: conf}

	return cloner.Clone(src)
}
