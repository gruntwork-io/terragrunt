package view_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/strict/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRender struct {
	listFn   func(strict.Controls) (string, error)
	detailFn func(strict.Control) (string, error)
}

func (f *fakeRender) List(c strict.Controls) (string, error)         { return f.listFn(c) }
func (f *fakeRender) DetailControl(c strict.Control) (string, error) { return f.detailFn(c) }

type failingWriter struct{ err error }

func (f *failingWriter) Write(_ []byte) (int, error) { return 0, f.err }

func TestNewWriter(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	r := &fakeRender{}

	w := view.NewWriter(buf, r)
	require.NotNil(t, w)
}

func TestWriterList(t *testing.T) {
	t.Parallel()

	t.Run("happy path writes rendered output", func(t *testing.T) {
		t.Parallel()

		buf := new(bytes.Buffer)
		r := &fakeRender{
			listFn: func(strict.Controls) (string, error) { return "listed", nil },
		}

		w := view.NewWriter(buf, r)
		require.NoError(t, w.List(strict.Controls{}))
		assert.Equal(t, "listed", buf.String())
	})

	t.Run("propagates render error", func(t *testing.T) {
		t.Parallel()

		boomErr := errors.New("render boom")
		r := &fakeRender{
			listFn: func(strict.Controls) (string, error) { return "", boomErr },
		}

		w := view.NewWriter(new(bytes.Buffer), r)
		err := w.List(strict.Controls{})
		require.ErrorIs(t, err, boomErr)
	})
}

func TestWriterDetailControl(t *testing.T) {
	t.Parallel()

	t.Run("happy path writes rendered output", func(t *testing.T) {
		t.Parallel()

		buf := new(bytes.Buffer)
		r := &fakeRender{
			detailFn: func(strict.Control) (string, error) { return "detailed", nil },
		}

		w := view.NewWriter(buf, r)
		require.NoError(t, w.DetailControl(&controls.Control{Name: "any"}))
		assert.Equal(t, "detailed", buf.String())
	})

	t.Run("propagates render error", func(t *testing.T) {
		t.Parallel()

		boomErr := errors.New("render boom")
		r := &fakeRender{
			detailFn: func(strict.Control) (string, error) { return "", boomErr },
		}

		w := view.NewWriter(new(bytes.Buffer), r)
		err := w.DetailControl(&controls.Control{Name: "any"})
		require.ErrorIs(t, err, boomErr)
	})
}

func TestWriterOutputError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("write boom")

	t.Run("List output error wrapped", func(t *testing.T) {
		t.Parallel()

		r := &fakeRender{
			listFn: func(strict.Controls) (string, error) { return "data", nil },
		}
		w := view.NewWriter(&failingWriter{err: sentinel}, r)

		err := w.List(strict.Controls{})
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("DetailControl output error wrapped", func(t *testing.T) {
		t.Parallel()

		r := &fakeRender{
			detailFn: func(strict.Control) (string, error) { return "data", nil },
		}
		w := view.NewWriter(&failingWriter{err: sentinel}, r)

		err := w.DetailControl(&controls.Control{Name: "any"})
		require.ErrorIs(t, err, sentinel)
	})
}
