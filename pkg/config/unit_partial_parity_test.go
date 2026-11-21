package config_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// partialParityCase is a decode list Terragrunt partially parses configs with, and the parts of the config its callers
// read.
type partialParityCase struct {
	// configure applies the decode list and the settings that come with it.
	configure func(*config.ParsingContext) *config.ParsingContext
	// read returns the parts of the config the callers of the decode list read.
	read func(*config.TerragruntConfig) any
}

// partialParityCases are the decode lists Terragrunt partially parses configs with outside discovery.
var partialParityCases = map[string]partialParityCase{
	"dependency flags": {
		configure: func(p *config.ParsingContext) *config.ParsingContext {
			return p.WithDecodeList(config.TerragruntFlags).WithDiagnosticsSuppressed()
		},
		read: readFlags,
	},
	"dependency flags and outputs": {
		configure: func(p *config.ParsingContext) *config.ParsingContext {
			return p.WithDecodeList(config.TerragruntFlags, config.OutputsBlock).
				WithDiagnosticsSuppressed().
				WithSkipOutputsResolution()
		},
		read: func(c *config.TerragruntConfig) any {
			return []any{readFlags(c), c.Outputs}
		},
	},
	"output fetch target": {
		configure: func(p *config.ParsingContext) *config.ParsingContext {
			return p.WithDecodeList(
				config.DependencyBlock,
				config.TerraformExtraArgs,
				config.TerragruntVersionConstraints,
			).WithDiagnosticsSuppressed()
		},
		read: func(c *config.TerragruntConfig) any {
			return []any{
				c.TerragruntDependencies,
				readSource(c),
				readExtraArgs(c),
				readVersionConstraints(c),
			}
		},
	},
	"output fetch remote state": {
		configure: func(p *config.ParsingContext) *config.ParsingContext {
			return p.WithDiagnosticsDiscarded().WithDecodeList(
				config.RemoteStateBlock,
				config.TerragruntFlags,
				config.EngineBlock,
			)
		},
		read: func(c *config.TerragruntConfig) any {
			return []any{c.RemoteState, readFlags(c), c.Engine}
		},
	},
	"terraform source": {
		configure: func(p *config.ParsingContext) *config.ParsingContext {
			return p.WithDecodeList(config.TerraformSource)
		},
		read: readSource,
	},
	"quiet terraform source": {
		configure: func(p *config.ParsingContext) *config.ParsingContext {
			return p.WithDecodeList(config.TerraformSource).WithDiagnosticsSuppressed()
		},
		read: readSource,
	},
	"dependency paths": {
		configure: func(p *config.ParsingContext) *config.ParsingContext {
			return p.WithDecodeList(config.DependencyBlock).WithDiagnosticsSuppressed()
		},
		read: func(c *config.TerragruntConfig) any {
			return c.TerragruntDependencies
		},
	},
	"backend": {
		configure: func(p *config.ParsingContext) *config.ParsingContext {
			return p.WithDiagnosticsSuppressed().WithDecodeList(config.RemoteStateBlock, config.TerraformSource)
		},
		read: func(c *config.TerragruntConfig) any {
			return []any{c.RemoteState, readSource(c)}
		},
	},
	"version check": {
		configure: func(p *config.ParsingContext) *config.ParsingContext {
			return p.WithDecodeList(
				config.TerragruntVersionConstraints,
				config.FeatureFlagsBlock,
				config.EngineBlock,
			)
		},
		read: func(c *config.TerragruntConfig) any {
			return []any{readVersionConstraints(c), c.FeatureFlags, c.Engine}
		},
	},
}

// partialParityFixtureRoots are the parity fixture roots and those whose configs hold remote_state blocks.
var partialParityFixtureRoots = append(
	slices.Clone(parityFixtureRoots),
	"codegen",
	"get-output",
	"dependency-include-remote-state",
	"s3-backend",
	"s3-encryption",
	"tofu-state-encryption",
)

// partialDivergentFixtures are fixtures that every decode list parses without error and
// [config.Store.ResolvePartialConfig] does not. The Store decodes every queue part, so a broken part no decode list
// names fails it.
var partialDivergentFixtures = map[string]struct{}{
	// The exclude block reads a dependency output, which discovery already rejects.
	"exclude/dependency-output/unit": {},
	// The exclude block holds a null where it needs a string.
	"exclude/null-string/null-string": {},
	// The included root's dependencies block reads a file that does not exist.
	"partial-parse/ignore-bad-block-in-parent/child": {},
}

func TestResolvePartialConfigMatchesPartialParseFixtures(t *testing.T) {
	t.Parallel()

	fixtures := loadFixtures(t, partialParityFixtureRoots, parityExcludedDirs)
	l := logger.CreateLogger()

	for name, tc := range partialParityCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, fixture := range fixtures.configs {
				t.Run(fixture.name, func(t *testing.T) {
					t.Parallel()

					wantCtx, wantRun := newTestUnitParsingContext(t, fixture.path)

					want, wantErr := config.PartialParseConfigFile(
						wantCtx,
						l,
						venv.MemVenv().WithFS(fixtures.newFS(t)),
						new(config.Store),
						tc.configure(wantRun),
						fixture.path,
						nil,
					)
					if wantErr != nil {
						return
					}

					gotCtx, gotRun := newTestUnitParsingContext(t, fixture.path)
					gotV := venv.MemVenv().WithFS(fixtures.newFS(t))
					got, gotErr := new(config.Store).ResolvePartialConfig(
						gotCtx, l, gotV, config.NewParseContext(gotV, tc.configure(gotRun)), fixture.path,
					)

					if _, divergent := partialDivergentFixtures[fixture.name]; divergent {
						require.Error(t, gotErr)

						return
					}

					require.NoError(t, gotErr)
					require.NotNil(t, got)
					assert.Equal(t, tc.read(want), tc.read(got))
				})
			}
		})
	}
}

// readFlags returns the flags the TerragruntFlags section decodes.
func readFlags(c *config.TerragruntConfig) any {
	return []any{c.PreventDestroy, c.IamRole, c.IamWebIdentityToken}
}

// readSource returns the terraform source, empty for a config with no terraform block.
func readSource(c *config.TerragruntConfig) any {
	if c.Terraform == nil {
		return nil
	}

	return c.Terraform.Source
}

// readExtraArgs returns the terraform extra arguments, nil for a config with no terraform block.
func readExtraArgs(c *config.TerragruntConfig) any {
	if c.Terraform == nil {
		return nil
	}

	return c.Terraform.ExtraArgs
}

// readVersionConstraints returns what the TerragruntVersionConstraints section decodes.
func readVersionConstraints(c *config.TerragruntConfig) any {
	return []any{c.TerragruntVersionConstraint, c.TerraformVersionConstraint, c.TerraformBinary}
}
