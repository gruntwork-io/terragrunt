package plaintext

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/stretchr/testify/require"
)

// panickyControl satisfies [strict.Control] but panics from GetName and
// GetSubcontrols. text/template recovers the panic and returns it as an
// Execute error, which is how these tests reach the error paths in
// [Render.List] and [Render.DetailSubcontrols].
type panickyControl struct{}

func (panickyControl) GetName() string                  { panic("boom") }
func (panickyControl) GetDescription() string           { return "" }
func (panickyControl) GetStatus() strict.Status         { return strict.ActiveStatus }
func (panickyControl) Enable()                          {}
func (panickyControl) GetEnabled() bool                 { return false }
func (panickyControl) GetSubcontrols() strict.Controls  { panic("boom") }
func (panickyControl) AddSubcontrols(...strict.Control) {}
func (panickyControl) SuppressWarning()                 {}
func (panickyControl) Evaluate(context.Context) error   { return nil }

func TestRenderListExecuteError(t *testing.T) {
	t.Parallel()

	r := NewRender()
	_, err := r.List(strict.Controls{panickyControl{}})
	require.Error(t, err)
}

func TestRenderDetailSubcontrolsExecuteError(t *testing.T) {
	t.Parallel()

	r := NewRender()
	_, err := r.DetailSubcontrols(strict.Controls{panickyControl{}})
	require.Error(t, err)
}

type failingFlusher struct {
	io.Writer
	err error
}

func (f *failingFlusher) Flush() error { return f.err }

// This test must run serially because it swaps the package-level newTabFlusher
// seam to force a Flush failure path. Other tests read that variable
// concurrently when running with t.Parallel().
//
//nolint:paralleltest // mutates package-level newTabFlusher.
func TestRenderFormatOutputFlushError(t *testing.T) {
	sentinel := errors.New("flush boom")
	original := newTabFlusher

	t.Cleanup(func() { newTabFlusher = original })

	newTabFlusher = func(w io.Writer) tabFlusher {
		return &failingFlusher{Writer: w, err: sentinel}
	}

	r := NewRender()
	_, err := r.List(strict.Controls{})
	require.ErrorIs(t, err, sentinel)
}
