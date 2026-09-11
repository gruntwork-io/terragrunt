// Package vbrowser provides a virtual web-browser abstraction for testing and
// production use.
//
// [Opener] hands a URL to whatever the operating system uses to open one.
// Production code builds the OS-backed opener via [NewOSOpener] and threads it
// down from [github.com/gruntwork-io/terragrunt/internal/venv.Venv]. Tests
// build an in-memory opener via [NewMemOpener], or [NewNoBrowserOpener] to
// assert that a code path opens nothing.
//
// An opener only ever launches an http or https URL. It is handed URLs that
// arrive over the network, and the launcher it runs treats other schemes as
// instructions: file: opens a local document and javascript: runs code in the
// browser the user is already signed into.
package vbrowser

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"time"
)

// ErrUnsupportedURL is returned for a URL that is not http or https.
var ErrUnsupportedURL = errors.New("vbrowser: only http and https URLs can be opened")

// openTimeout bounds the launcher, not the browsing that follows. A launcher
// returns as soon as it has handed the URL off, so reaching this means it hung
// rather than that the user is reading the page.
const openTimeout = 10 * time.Second

// Opener sends a URL to the user's web browser.
type Opener interface {
	// Open launches rawURL. A scheme other than http or https yields an error
	// wrapping [ErrUnsupportedURL]. A launcher that fails or is not installed
	// is reported too, so the caller can fall back to telling the user to open
	// the URL themselves.
	Open(ctx context.Context, rawURL string) error
}

// Handler opens one URL for the in-memory backend. It is invoked synchronously
// by an [Opener] returned from [NewMemOpener].
type Handler func(ctx context.Context, rawURL string) error

// NewOSOpener returns an [Opener] that runs the host's URL launcher.
func NewOSOpener() Opener {
	return osOpener{goos: runtime.GOOS}
}

// NewMemOpener returns an [Opener] whose every call is dispatched to h instead
// of launching anything. h must not be nil.
func NewMemOpener(h Handler) Opener {
	if h == nil {
		panic("vbrowser: NewMemOpener requires a non-nil Handler")
	}

	return memOpener{handler: h}
}

// OpenCommand returns the launcher that opens rawURL on goos, as a program name
// and its arguments. It is exported so the per-platform choice can be tested
// from any host, since Terragrunt ships for macOS, Windows, and Linux.
func OpenCommand(goos, rawURL string) (string, []string, error) {
	if err := checkURL(rawURL); err != nil {
		return "", nil, err
	}

	switch goos {
	case "darwin":
		return "open", []string{rawURL}, nil
	case "windows":
		// rundll32 rather than `cmd /c start`, which reads &, ^, and % in the
		// URL as shell syntax and needs an empty title argument to boot.
		return "rundll32.exe", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	default:
		return "xdg-open", []string{rawURL}, nil
	}
}

// checkURL rejects what must never reach a launcher. The scheme test also
// guarantees the URL cannot start with a dash, which a launcher would read as
// a flag of its own rather than as the page to open.
func checkURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("vbrowser: parsing %q: %w", rawURL, err)
	}

	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("%w: %q", ErrUnsupportedURL, rawURL)
	}

	return nil
}

type memOpener struct {
	handler Handler
}

func (o memOpener) Open(ctx context.Context, rawURL string) error {
	if err := checkURL(rawURL); err != nil {
		return err
	}

	return o.handler(ctx, rawURL)
}
