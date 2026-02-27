// Package creds provides a way to obtain credentials through different providers and set them to `opts.Env`.
package creds

import (
	"context"
	"maps"

	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// shellRunOptsFromOpts constructs shell.RunOptions from TerragruntOptions.
// This is a local helper to avoid an import cycle with configbridge.
func shellRunOptsFromOpts(opts *options.TerragruntOptions) *shell.RunOptions {
	return &shell.RunOptions{
		WorkingDir:              opts.WorkingDir,
		Writer:                  opts.Writer,
		ErrWriter:               opts.ErrWriter,
		Env:                     opts.Env,
		TFPath:                  opts.TFPath,
		Engine:                  opts.Engine,
		Experiments:             opts.Experiments,
		NoEngine:                opts.NoEngine,
		Telemetry:               opts.Telemetry,
		RootWorkingDir:          opts.RootWorkingDir,
		LogShowAbsPaths:         opts.LogShowAbsPaths,
		LogDisableErrorSummary:  opts.LogDisableErrorSummary,
		Headless:                opts.Headless,
		ForwardTFStdout:         opts.ForwardTFStdout,
		EngineCachePath:         opts.EngineCachePath,
		EngineLogLevel:          opts.EngineLogLevel,
		EngineSkipChecksumCheck: opts.EngineSkipChecksumCheck,
	}
}

type Getter struct {
	obtainedCreds map[string]*providers.Credentials
}

func NewGetter() *Getter {
	return &Getter{
		obtainedCreds: make(map[string]*providers.Credentials),
	}
}

// ObtainAndUpdateEnvIfNecessary obtains credentials through different providers and sets them to the provided env map.
func (getter *Getter) ObtainAndUpdateEnvIfNecessary(ctx context.Context, l log.Logger, env map[string]string, authProviders ...providers.Provider) error {
	for _, provider := range authProviders {
		creds, err := provider.GetCredentials(ctx, l)
		if err != nil {
			return err
		}

		if creds == nil {
			continue
		}

		for providerName, prevCreds := range getter.obtainedCreds {
			if prevCreds.Name == creds.Name {
				l.Warnf("%s credentials obtained using %s are overwritten by credentials obtained using %s.", creds.Name, providerName, provider.Name())
			}
		}

		getter.obtainedCreds[provider.Name()] = creds

		maps.Copy(env, creds.Envs)
	}

	return nil
}

// ObtainCredsForParsing creates a new Getter, obtains external-command
// credentials, and populates env before HCL parsing.
// Use when sops_decrypt_file() or get_aws_account_id() may appear in locals.
// See https://github.com/gruntwork-io/terragrunt/issues/5515
func ObtainCredsForParsing(ctx context.Context, l log.Logger, authProviderCmd string, env map[string]string, opts *options.TerragruntOptions) (*Getter, error) {
	g := NewGetter()
	if err := g.ObtainAndUpdateEnvIfNecessary(ctx, l, env, externalcmd.NewProvider(l, authProviderCmd, shellRunOptsFromOpts(opts))); err != nil {
		return nil, err
	}

	return g, nil
}
