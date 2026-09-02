package login

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/portal"
	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// loginUnavailableCodes are the refusals of a login request that retrying will
// not clear.
var loginUnavailableCodes = []portal.ErrorCode{
	portal.ErrorCodeInvalidClient,
	portal.ErrorCodeInvalidScope,
	portal.ErrorCodeFeatureNotEnabled,
}

// Run signs the user in to the Gruntwork Developer Portal and files the
// credential it issues. A credential an earlier login left behind ends the run
// instead, unless [Options.Force] is set.
func Run(ctx context.Context, l log.Logger, v *venv.Venv, opts *Options) error {
	v.RequireHTTP()
	v.RequireWriters()

	w := v.Writers.Writer

	current, err := portal.LoadTokens(l, v, opts.BaseURL)
	if err != nil {
		return err
	}

	if len(current) > 0 && !opts.Force {
		return reportCurrentLogins(w, current, Command(opts.Experiments))
	}

	auth, err := portal.AuthorizeDevice(ctx, l, v.HTTP, opts.BaseURL)
	if err != nil {
		return startFailure(err)
	}

	if err := portal.PromptApproval(ctx, l, v, auth); err != nil {
		return err
	}

	var token *portal.Token

	poll := func() error {
		var err error

		token, err = portal.PollToken(ctx, l, v.HTTP, opts.BaseURL, auth)

		return err
	}

	msgs := spinner.Messages{
		Working: "Waiting for you to approve the login",
		Done:    "Approved",
	}

	if err := spinner.Show(ctx, l, spinner.Writer(v), msgs, poll); err != nil {
		return approvalFailure(err, Command(opts.Experiments))
	}

	if err := portal.SaveToken(l, v, opts.BaseURL, token); err != nil {
		return err
	}

	return writeLine(w, "Signed in "+describe(token.Account, token.Org))
}

// reportCurrentLogins names the credentials an earlier login left behind, and
// the flag that replaces them.
func reportCurrentLogins(w io.Writer, tokens map[string]portal.StoredToken, command string) error {
	sorted := slices.SortedFunc(maps.Values(tokens), func(a, b portal.StoredToken) int {
		return cmp.Or(cmp.Compare(a.Org.Name, b.Org.Name), cmp.Compare(a.Org.ID, b.Org.ID))
	})

	for _, token := range sorted {
		if err := writeLine(w, "Already signed in "+describe(token.Account, token.Org)); err != nil {
			return err
		}
	}

	return writeLine(w, "Run `"+command+" --"+ForceFlagName+"` to sign in again.")
}

// describe names what a credential reaches, narrowing to the organization alone
// when the portal named no account.
func describe(account portal.Account, org portal.Org) string {
	name := org.Name
	if name == "" {
		name = org.ID
	}

	if account.Email == "" {
		return "to " + name
	}

	return "as " + account.Email + " — " + name
}

// approvalFailure names the command that starts a new login when the one the
// user left unanswered ran out of time. Any other error passes through.
func approvalFailure(err error, command string) error {
	if !errors.Is(err, portal.ErrLoginExpired) {
		return err
	}

	return fmt.Errorf("%w; run `%s` to start a new one", err, command)
}

// startFailure reports a refusal in loginUnavailableCodes as a portal that is
// not accepting logins, rather than as a raw code the user would have to look
// up.
func startFailure(err error) error {
	var portalErr *portal.Error
	if !errors.As(err, &portalErr) {
		return err
	}

	if !slices.Contains(loginUnavailableCodes, portalErr.Code) {
		return err
	}

	return fmt.Errorf("%w: %w", ErrLoginUnavailable, err)
}

func writeLine(w io.Writer, line string) error {
	if _, err := fmt.Fprintln(w, line); err != nil {
		return fmt.Errorf("writing the login status: %w", err)
	}

	return nil
}
