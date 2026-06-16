package tui_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSourceLoadErrorAllFailed pins the boundary between "every source
// failed" (terminal for the session) and a partial failure (the catalog
// stays usable).
func TestSourceLoadErrorAllFailed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  *tui.SourceLoadError
		name string
		want bool
	}{
		{
			name: "all sources failed",
			err: &tui.SourceLoadError{
				Failures: []tui.SourceFailure{
					{URL: "github.com/acme/a", Err: errors.New("boom")},
					{URL: "github.com/acme/b", Err: errors.New("boom")},
				},
				Attempted: 2,
			},
			want: true,
		},
		{
			name: "partial failure",
			err: &tui.SourceLoadError{
				Failures: []tui.SourceFailure{
					{URL: "github.com/acme/a", Err: errors.New("boom")},
				},
				Attempted: 3,
			},
			want: false,
		},
		{
			name: "no sources attempted",
			err: &tui.SourceLoadError{
				Failures:  nil,
				Attempted: 0,
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.err.AllFailed())
		})
	}
}

// TestSourceLoadErrorUnwrapExposesCauses verifies that errors.Is can find
// any per-source cause through the aggregate.
func TestSourceLoadErrorUnwrapExposesCauses(t *testing.T) {
	t.Parallel()

	first := errors.New("clone failed")
	second := errors.New("authentication required")

	err := &tui.SourceLoadError{
		Failures: []tui.SourceFailure{
			{URL: "github.com/acme/a", Err: first},
			{URL: "github.com/acme/b", Err: second},
		},
		Attempted: 2,
	}

	require.ErrorIs(t, err, first)
	require.ErrorIs(t, err, second)
	require.NotErrorIs(t, err, errors.New("unrelated"))
}

// TestSourceLoadErrorSummary pins the one-line summary the TUI renders in
// the list-view notice and the welcome error screen.
func TestSourceLoadErrorSummary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  *tui.SourceLoadError
		name string
		want string
	}{
		{
			name: "all failed plural",
			err: &tui.SourceLoadError{
				Failures: []tui.SourceFailure{
					{URL: "a", Err: errors.New("boom")},
					{URL: "b", Err: errors.New("boom")},
				},
				Attempted: 2,
			},
			want: "failed to load all 2 catalog sources",
		},
		{
			name: "all failed singular",
			err: &tui.SourceLoadError{
				Failures: []tui.SourceFailure{
					{URL: "a", Err: errors.New("boom")},
				},
				Attempted: 1,
			},
			want: "failed to load all 1 catalog source",
		},
		{
			name: "partial failure",
			err: &tui.SourceLoadError{
				Failures: []tui.SourceFailure{
					{URL: "a", Err: errors.New("boom")},
				},
				Attempted: 3,
			},
			want: "failed to load 1 of 3 catalog sources",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.err.Error())
		})
	}
}
