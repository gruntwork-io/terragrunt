// Package creds provides a way to obtain credentials through different providers and set them to `v.Env`.
package creds

import (
	"context"
	"maps"

	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type Getter struct {
	obtainedCreds map[string]*providers.Credentials
}

func NewGetter() *Getter {
	return &Getter{
		obtainedCreds: make(map[string]*providers.Credentials),
	}
}

// ObtainAndUpdateEnvIfNecessary obtains credentials through different providers and sets them on v.Env.
//
// Requires v.Env: obtained credentials are written into it.
func (getter *Getter) ObtainAndUpdateEnvIfNecessary(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	authProviders ...providers.Provider,
) error {
	v.RequireEnv()

	for _, provider := range authProviders {
		creds, err := provider.GetCredentials(ctx, l, v)
		if err != nil {
			return err
		}

		if creds == nil {
			continue
		}

		for providerName, prevCreds := range getter.obtainedCreds {
			if prevCreds.Name == creds.Name {
				l.Warnf("%s credentials obtained using %s are overwritten by credentials obtained using %s.",
					creds.Name, providerName, provider.Name())
			}
		}

		getter.obtainedCreds[provider.Name()] = creds

		maps.Copy(v.Env, creds.Envs)
	}

	return nil
}

// ObtainCredsForParsing creates a new Getter, obtains external-command
// credentials, and populates v.Env before HCL parsing.
// Use when sops_decrypt_file() or get_aws_account_id() may appear in locals.
// See https://github.com/gruntwork-io/terragrunt/issues/5515
//
// Requires a non-nil v.Env.
func ObtainCredsForParsing(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	authProviderCmd string,
	shellOpts *shell.ShellOptions,
) (*Getter, error) {
	g := NewGetter()

	provider := externalcmd.NewProvider(l, authProviderCmd, shellOpts)
	if err := g.ObtainAndUpdateEnvIfNecessary(ctx, l, v, provider); err != nil {
		return nil, err
	}

	return g, nil
}
