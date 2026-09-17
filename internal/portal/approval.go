package portal

import (
	"context"
	"fmt"
	"io"

	"charm.land/lipgloss/v2"

	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// PromptApproval shows the user the code the approval page will ask them to
// confirm, then sends their browser to that page.
//
// When the browser does not open, the user visits the URL and enters the code
// to approve the login.
func PromptApproval(ctx context.Context, l log.Logger, v *venv.Venv, auth *DeviceAuthorization) error {
	v.RequireWriters()
	v.RequireBrowser()

	w := v.Writers.Writer
	target := auth.approvalURL()
	style := newApprovalStyle(stdout.ShouldColor(l, v))

	if err := writeLines(w,
		"Your one-time code: "+style.code(auth.UserCode),
		"Opening your browser to "+style.url(target),
	); err != nil {
		return err
	}

	if err := v.Browser.Open(ctx, target); err != nil {
		l.Debugf("Could not open a browser: %v", err)

		return writeLines(w, style.hint("  No browser opened. Visit the URL above and enter the code."))
	}

	return writeLines(w, style.hint("  Didn't open? Visit the URL above and enter the code."))
}

// ANSI palette indexes rather than hex values, so each color is the one the
// user's terminal theme assigns.
const (
	ansiBrightCyan = "14"
	ansiBrightBlue = "12"
)

// approvalStyle draws each part of the prompt by what the user does with it.
// The code is the only part they type somewhere else, so it is the only part
// emphasized.
type approvalStyle struct {
	code func(string) string
	url  func(string) string
	hint func(string) string
}

// newApprovalStyle leaves every part unstyled when the escape sequences would
// land in a pipe or a file as text.
func newApprovalStyle(shouldColor bool) approvalStyle {
	if !shouldColor {
		return approvalStyle{
			code: func(s string) string { return s },
			url:  func(s string) string { return s },
			hint: func(s string) string { return s },
		}
	}

	codeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ansiBrightCyan))
	urlStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ansiBrightBlue))
	hintStyle := lipgloss.NewStyle().Faint(true)

	return approvalStyle{
		code: func(s string) string { return codeStyle.Render(s) },
		url:  func(s string) string { return urlStyle.Render(s) },
		hint: func(s string) string { return hintStyle.Render(s) },
	}
}

// approvalURL is the page to send the user to. The portal's pre-filled form
// leaves them nothing to type, so it wins when the portal supplied one.
func (a *DeviceAuthorization) approvalURL() string {
	if a.VerificationURIComplete != "" {
		return a.VerificationURIComplete
	}

	return a.VerificationURI
}

func writeLines(w io.Writer, lines ...string) error {
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return fmt.Errorf("writing the login approval prompt: %w", err)
		}
	}

	return nil
}
