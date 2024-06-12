package creds

import (
	"context"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/options"
	"golang.org/x/exp/maps"
)

// ObtainCredentialsAndUpdateEnvIfNecessary obtains credentials through different providers and sets them to `opts.Env`.
func ObtainCredentialsAndUpdateEnvIfNecessary(ctx context.Context, opts *options.TerragruntOptions) error {
	authProviders := []providers.Provider{
		externalcmd.NewProvider(opts),
		amazonsts.NewProvider(opts),
	}

	obtainedCreds := make(map[string]*providers.Credentials)

	for _, provider := range authProviders {
		creds, err := provider.GetCredentials(ctx)
		if err != nil {
			return err
		}
		if creds == nil {
			continue
		}

		for providerName, prevCreds := range obtainedCreds {
			if prevCreds.Name == creds.Name {
				opts.Logger.Warnf("%s credentials obtained using %s are overwritten by credentials obtained using %s.", creds.Name, providerName, provider.Name())
			}
		}
		obtainedCreds[provider.Name()] = creds

		maps.Copy(opts.Env, creds.Envs)
	}

	return nil
}
