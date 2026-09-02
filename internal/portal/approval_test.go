package portal_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/portal"
	"github.com/gruntwork-io/terragrunt/internal/vbrowser"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	approvalURI         = "https://portal.example.com/auth/device"
	approvalURIComplete = "https://portal.example.com/auth/device?user_code=FAKE-CODE"
)

func newAuthorization() *portal.DeviceAuthorization {
	return &portal.DeviceAuthorization{
		DeviceCode:              portal.Secret("fake-device-code"),
		UserCode:                "FAKE-CODE",
		VerificationURI:         approvalURI,
		VerificationURIComplete: approvalURIComplete,
	}
}

func TestPromptApprovalOpensPrefilledURL(t *testing.T) {
	t.Parallel()

	var (
		out    bytes.Buffer
		opened string
	)

	v := venvtest.New()
	v.Writers = &writer.Writers{Writer: &out, ErrWriter: &out}
	v = v.WithBrowser(vbrowser.NewMemOpener(func(_ context.Context, rawURL string) error {
		opened = rawURL

		return nil
	}))

	require.NoError(t, portal.PromptApproval(t.Context(), logger.CreateLogger(), v, newAuthorization()))

	assert.Equal(t, approvalURIComplete, opened)
	assert.Contains(t, out.String(), "FAKE-CODE")
	assert.Contains(t, out.String(), approvalURIComplete)
}

// TestPromptApprovalPrintsBeforeOpening pins the ordering: the code reaches the
// terminal before a browser window can take focus and cover it.
func TestPromptApprovalPrintsBeforeOpening(t *testing.T) {
	t.Parallel()

	var (
		out           bytes.Buffer
		printedAtOpen string
	)

	v := venvtest.New()
	v.Writers = &writer.Writers{Writer: &out, ErrWriter: &out}
	v = v.WithBrowser(vbrowser.NewMemOpener(func(context.Context, string) error {
		printedAtOpen = out.String()

		return nil
	}))

	require.NoError(t, portal.PromptApproval(t.Context(), logger.CreateLogger(), v, newAuthorization()))

	assert.Contains(t, printedAtOpen, "FAKE-CODE")
	assert.Contains(t, printedAtOpen, approvalURIComplete)
}

// TestPromptApprovalFallsBackToPlainURL pins that the portal omitting the
// pre-filled URL still sends the user somewhere they can type the code.
func TestPromptApprovalFallsBackToPlainURL(t *testing.T) {
	t.Parallel()

	var (
		out    bytes.Buffer
		opened string
	)

	auth := newAuthorization()
	auth.VerificationURIComplete = ""

	v := venvtest.New()
	v.Writers = &writer.Writers{Writer: &out, ErrWriter: &out}
	v = v.WithBrowser(vbrowser.NewMemOpener(func(_ context.Context, rawURL string) error {
		opened = rawURL

		return nil
	}))

	require.NoError(t, portal.PromptApproval(t.Context(), logger.CreateLogger(), v, auth))

	assert.Equal(t, approvalURI, opened)
	assert.Contains(t, out.String(), approvalURI)
}

// TestPromptApprovalSurvivesNoBrowser pins the headless path: a host that
// cannot open a browser still gets the code and the URL, and login carries on
// so the user can approve from another device.
func TestPromptApprovalSurvivesNoBrowser(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	v := venvtest.New()
	v.Writers = &writer.Writers{Writer: &out, ErrWriter: &out}

	require.NoError(t, portal.PromptApproval(t.Context(), logger.CreateLogger(), v, newAuthorization()))

	assert.Contains(t, out.String(), "FAKE-CODE")
	assert.Contains(t, out.String(), approvalURIComplete)
	assert.Contains(t, out.String(), "No browser opened")
}

// TestPromptApprovalDoesNotPrintDeviceCode pins that the credential stays out
// of the terminal while the user code, which the user must read, does not.
func TestPromptApprovalDoesNotPrintDeviceCode(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	v := venvtest.New()
	v.Writers = &writer.Writers{Writer: &out, ErrWriter: &out}
	v = v.WithBrowser(vbrowser.NewMemOpener(func(context.Context, string) error { return nil }))

	require.NoError(t, portal.PromptApproval(t.Context(), logger.CreateLogger(), v, newAuthorization()))

	assert.NotContains(t, out.String(), "fake-device-code")
	assert.Contains(t, out.String(), "FAKE-CODE")
}

func TestPromptApprovalReportsWriteFailure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("broken pipe")

	v := venvtest.New()
	v.Writers = &writer.Writers{Writer: failingWriter{err: sentinel}, ErrWriter: &bytes.Buffer{}}
	v = v.WithBrowser(vbrowser.NewMemOpener(func(context.Context, string) error { return nil }))

	err := portal.PromptApproval(t.Context(), logger.CreateLogger(), v, newAuthorization())
	require.ErrorIs(t, err, sentinel)
}

// TestPromptApprovalRejectsUnbrowsableURL pins that the opener refuses a
// verification URL the portal should never have sent, even though the parse in
// AuthorizeDevice already turns one away.
func TestPromptApprovalRejectsUnbrowsableURL(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	auth := newAuthorization()
	auth.VerificationURIComplete = "file:///fake/path"

	v := venvtest.New()
	v.Writers = &writer.Writers{Writer: &out, ErrWriter: &out}
	v = v.WithBrowser(vbrowser.NewMemOpener(func(context.Context, string) error {
		t.Error("the handler must not be reached for an unbrowsable URL")

		return nil
	}))

	require.NoError(t, portal.PromptApproval(t.Context(), logger.CreateLogger(), v, auth))
	assert.Contains(t, out.String(), "No browser opened")
}

// TestPromptApprovalStylesNothingWithoutATerminal pins what a pipe or a log
// file receives: text with no escape sequences in it.
func TestPromptApprovalStylesNothingWithoutATerminal(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	v := venvtest.New()
	v.Writers = &writer.Writers{Writer: &out, ErrWriter: &out}
	v = v.WithBrowser(vbrowser.NewMemOpener(func(context.Context, string) error { return nil }))

	require.NoError(t, portal.PromptApproval(t.Context(), logger.CreateLogger(), v, newAuthorization()))

	assert.NotContains(t, out.String(), escape)
	assert.Contains(t, out.String(), "Your one-time code: FAKE-CODE")
}

// TestPromptApprovalDrawsTheEyeToTheCode pins what a terminal receives: the
// code and the URL drawn differently, and the code still readable as plain
// text for copying.
func TestPromptApprovalDrawsTheEyeToTheCode(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	v := venvtest.New()
	v.Writers = &writer.Writers{Writer: &out, ErrWriter: &out}
	v.Terminal = &venv.Terminal{
		StdinIsTTY:  func() bool { return false },
		StdoutIsTTY: func() bool { return true },
		StderrIsTTY: func() bool { return false },
		Width:       func() int { return 80 },
	}
	v = v.WithBrowser(vbrowser.NewMemOpener(func(context.Context, string) error { return nil }))

	// The shared logger disables colors, which every other test here wants.
	l := logger.CreateLogger()
	l.Formatter().SetDisabledColors(false)

	require.NoError(t, portal.PromptApproval(t.Context(), l, v, newAuthorization()))

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 3)

	codeLine, urlLine, hintLine := lines[0], lines[1], lines[2]

	assert.Contains(t, codeLine, escape, "the code carries styling")
	assert.Contains(t, stripEscapes(codeLine), "FAKE-CODE", "the code survives as copyable text")

	assert.Contains(t, urlLine, escape, "the URL is marked as a URL")
	assert.Contains(t, stripEscapes(urlLine), approvalURIComplete)

	// A terminal finds the URL to make clickable by scanning for an unbroken
	// run of it.
	assert.Contains(t, urlLine, approvalURIComplete, "the URL survives as one unbroken run")

	assert.Contains(t, hintLine, escape, "the hint recedes")
}

// escape opens every ANSI styling sequence lipgloss writes.
const escape = "\x1b"

// stripEscapes removes ANSI sequences so an assertion reads the text a user
// would see rather than how it was drawn.
func stripEscapes(s string) string {
	var b strings.Builder

	for {
		start := strings.Index(s, escape)
		if start < 0 {
			b.WriteString(s)

			return b.String()
		}

		b.WriteString(s[:start])

		end := strings.IndexByte(s[start:], 'm')
		if end < 0 {
			return b.String()
		}

		s = s[start+end+1:]
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}
