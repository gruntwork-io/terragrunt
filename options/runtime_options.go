package options

import (
	"context"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/hashicorp/go-version"
	"github.com/puzpuzpuz/xsync/v3"
)

// RuntimeOptions is the lean subset of options needed for execution (runner/download/error handling).
// These fields were previously scattered across TerragruntOptions; grouping them here lets downstream
// consumers copy a single struct instead of mirroring individual fields.
type RuntimeOptions struct {
	Writer                       io.Writer
	ErrWriter                    io.Writer
	TerragruntVersion            *version.Version             `clone:"shadowcopy"`
	FeatureFlags                 *xsync.MapOf[string, string] `clone:"shadowcopy"`
	Engine                       *EngineOptions
	Telemetry                    *telemetry.Options
	RunTerragrunt                func(ctx context.Context, l log.Logger, opts *TerragruntOptions, r *report.Report) error
	TerraformVersion             *version.Version `clone:"shadowcopy"`
	Errors                       *ErrorsConfig
	Env                          map[string]string
	OutputFolder                 string
	OriginalTerragruntConfigPath string
	RootWorkingDir               string
	JSONOutputFolder             string
	TerragruntConfigPath         string
	Source                       string
	WorkingDir                   string
	DownloadDir                  string
	TFPath                       string
	TerraformImplementation      TerraformImplementationType
	TerraformCommand             string
	OriginalTerraformCommand     string
	IAMRoleOptions               IAMRoleOptions
	OriginalIAMRoleOptions       IAMRoleOptions
	VersionManagerFileName       []string
	Experiments                  experiment.Experiments `clone:"shadowcopy"`
	TerraformCliArgs             cli.Args
	Parallelism                  int
	MaxFoldersToCheck            int
	SourceUpdate                 bool
	Debug                        bool
	Headless                     bool
	IgnoreDependencyErrors       bool
	IncludeExternalDependencies  bool
	CheckDependentModules        bool
	IgnoreDependencyOrder        bool
	RunAllAutoApprove            bool
	FailFast                     bool
	IgnoreExternalDependencies   bool
	ForwardTFStdout              bool
	JSONLogFormat                bool
	BackendBootstrap             bool
	EngineEnabled                bool
	AutoRetry                    bool
	AutoInit                     bool
	TFPathExplicitlySet          bool
	LogDisableErrorSummary       bool
	NonInteractive               bool
}

// Clone creates a shallow copy of RuntimeOptions (map/slice fields are manually copied where needed).
func (o *RuntimeOptions) Clone() *RuntimeOptions {
	if o == nil {
		return nil
	}

	copied := *o

	if o.Env != nil {
		copied.Env = make(map[string]string, len(o.Env))
		for k, v := range o.Env {
			copied.Env[k] = v
		}
	}

	if o.TerraformCliArgs != nil {
		copied.TerraformCliArgs = append(cli.Args{}, o.TerraformCliArgs...)
	}

	if o.VersionManagerFileName != nil {
		copied.VersionManagerFileName = append([]string{}, o.VersionManagerFileName...)
	}

	return &copied
}
