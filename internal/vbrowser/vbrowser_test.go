package vbrowser_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vbrowser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCommand(t *testing.T) {
	t.Parallel()

	tc := []struct {
		goos     string
		wantName string
		wantArgs []string
	}{
		{goos: "darwin", wantName: "open", wantArgs: []string{"https://portal.example.com/auth/device"}},
		{
			goos:     "windows",
			wantName: "rundll32.exe",
			wantArgs: []string{"url.dll,FileProtocolHandler", "https://portal.example.com/auth/device"},
		},
		{goos: "linux", wantName: "xdg-open", wantArgs: []string{"https://portal.example.com/auth/device"}},
		{goos: "freebsd", wantName: "xdg-open", wantArgs: []string{"https://portal.example.com/auth/device"}},
	}

	for _, tt := range tc {
		t.Run(tt.goos, func(t *testing.T) {
			t.Parallel()

			name, args, err := vbrowser.OpenCommand(tt.goos, "https://portal.example.com/auth/device")
			require.NoError(t, err)

			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestOpenCommandRejectsUnsupportedScheme pins the guard that stands between a
// URL arriving over the network and the launcher: a scheme other than http or
// https is an instruction to open a local document or run code, not a page.
func TestOpenCommandRejectsUnsupportedScheme(t *testing.T) {
	t.Parallel()

	for _, rawURL := range []string{
		"file:///fake/path",
		"javascript:fake-script",
		"ftp://portal.example.com",
		"portal.example.com/auth/device",
		"-fake-flag",
		"",
	} {
		t.Run(rawURL, func(t *testing.T) {
			t.Parallel()

			_, _, err := vbrowser.OpenCommand("linux", rawURL)
			require.ErrorIs(t, err, vbrowser.ErrUnsupportedURL)
		})
	}
}

func TestMemOpenerReceivesURL(t *testing.T) {
	t.Parallel()

	var got string

	o := vbrowser.NewMemOpener(func(_ context.Context, rawURL string) error {
		got = rawURL

		return nil
	})

	require.NoError(t, o.Open(t.Context(), "https://portal.example.com/auth/device"))
	assert.Equal(t, "https://portal.example.com/auth/device", got)
}

// TestMemOpenerChecksURLBeforeHandler pins that the scheme guard runs for the
// in-memory opener too, so a test cannot pass a URL the OS opener would refuse.
func TestMemOpenerChecksURLBeforeHandler(t *testing.T) {
	t.Parallel()

	called := false

	o := vbrowser.NewMemOpener(func(context.Context, string) error {
		called = true

		return nil
	})

	require.ErrorIs(t, o.Open(t.Context(), "file:///fake/path"), vbrowser.ErrUnsupportedURL)
	assert.False(t, called)
}

func TestMemOpenerPropagatesHandlerFailure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("no display")

	o := vbrowser.NewMemOpener(func(context.Context, string) error {
		return sentinel
	})

	require.ErrorIs(t, o.Open(t.Context(), "https://portal.example.com/auth/device"), sentinel)
}

func TestNoBrowserOpenerRefuses(t *testing.T) {
	t.Parallel()

	err := vbrowser.NewNoBrowserOpener().Open(t.Context(), "https://portal.example.com/auth/device")
	require.ErrorIs(t, err, vbrowser.ErrNoBrowser)
}

func TestNewMemOpenerRejectsNilHandler(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() { vbrowser.NewMemOpener(nil) })
}
