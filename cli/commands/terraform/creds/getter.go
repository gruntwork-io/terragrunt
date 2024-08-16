// Package creds provides a way to obtain credentials through different providers.
package creds

import (
	"context"
	"errors"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/options"
	"golang.org/x/exp/maps"
)

// Getter is a struct that contains the obtained credentials.
type Getter struct {
	obtainedCreds map[string]*providers.Credentials
}

// NewGetter returns a new instance of Getter.
func NewGetter() *Getter {
	return &Getter{
		obtainedCreds: make(map[string]*providers.Credentials),
	}
}

// ObtainAndUpdateEnvIfNecessary obtains credentials through different providers and sets them to `opts.Env`.
func (getter *Getter) ObtainAndUpdateEnvIfNecessary(ctx context.Context, opts *options.TerragruntOptions, authProviders ...providers.Provider) error { //nolint:lll
	for _, provider := range authProviders {
		creds, err := provider.GetCredentials(ctx)
		if errors.Is(err, amazonsts.ErrRoleNotDefined) || errors.Is(err, externalcmd.ErrCommandNotDefined) {
			continue
		}

		if err != nil {
			return err
		}

		for providerName, prevCreds := range getter.obtainedCreds {
			if prevCreds.Name == creds.Name {
				opts.Logger.Warnf(
					"%s credentials obtained using %s are overwritten by credentials obtained using %s.",
					creds.Name,
					providerName,
					provider.Name(),
				)
			}
		}

		getter.obtainedCreds[provider.Name()] = creds

		maps.Copy(opts.Env, creds.Envs)
	}

	return nil
}
