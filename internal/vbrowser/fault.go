package vbrowser

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoBrowser is returned by openers built from [NewNoBrowserOpener].
var ErrNoBrowser = errors.New("vbrowser: opening a browser is not permitted")

// NewNoBrowserOpener returns an [Opener] whose every call fails with an error
// wrapping [ErrNoBrowser]. It lets tests assert that a code path opens no
// browser, and stands in for the host that has none.
func NewNoBrowserOpener() Opener {
	return noBrowserOpener{}
}

type noBrowserOpener struct{}

func (noBrowserOpener) Open(_ context.Context, rawURL string) error {
	return fmt.Errorf("%w: %q", ErrNoBrowser, rawURL)
}
