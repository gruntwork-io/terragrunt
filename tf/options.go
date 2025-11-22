package tf

import (
	"io"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/telemetry"
)

// TFOptions contains all options needed for tf package operations.
// This is independent from TerragruntOptions and RunnerOptions, providing
// a focused interface for Terraform/OpenTofu command execution.
type TFOptions struct {
	Writer                  io.Writer
	ErrWriter               io.Writer
	Telemetry               *telemetry.Options
	Engine                  *options.EngineOptions
	Env                     map[string]string
	WorkingDir              string
	DownloadDir             string
	TerragruntConfigPath    string
	TFPath                  string
	TerraformImplementation options.TerraformImplementationType
	TerraformCliArgs        []string
	ForwardTFStdout         bool
	JSONLogFormat           bool
	Headless                bool
	LogDisableErrorSummary  bool
	EngineEnabled           bool
	TFPathExplicitlySet     bool
}

// Clone creates a shallow copy of TFOptions.
// Note: Maps and slices are not deep copied, so modifications to them will affect the original.
func (opts *TFOptions) Clone() *TFOptions {
	if opts == nil {
		return nil
	}

	clone := *opts

	// Deep clone the environment map
	if opts.Env != nil {
		clone.Env = make(map[string]string, len(opts.Env))
		for k, v := range opts.Env {
			clone.Env[k] = v
		}
	}

	// Deep clone TerraformCliArgs slice
	if opts.TerraformCliArgs != nil {
		clone.TerraformCliArgs = make([]string, len(opts.TerraformCliArgs))
		copy(clone.TerraformCliArgs, opts.TerraformCliArgs)
	}

	return &clone
}
