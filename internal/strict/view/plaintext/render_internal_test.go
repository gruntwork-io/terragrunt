package plaintext

import (
	"context"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/stretchr/testify/require"
)

// panickyControl satisfies the strict.Control interface but panics when its
// subcontrols are asked for. The text/template engine recovers the panic and
// surfaces it as an Execute error, which lets us drive the error-wrapping
// branches in List and DetailControl.
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

func TestRenderDetailControlExecuteError(t *testing.T) {
	t.Parallel()

	r := NewRender()
	_, err := r.DetailControl(panickyControl{})
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

	newTabFlusher = func(out io.Writer) tabFlusher {
		return &failingFlusher{Writer: out, err: sentinel}
	}

	r := NewRender()
	_, err := r.List(strict.Controls{})
	require.ErrorIs(t, err, sentinel)
}
