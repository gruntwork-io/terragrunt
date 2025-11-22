package run

import (
	runnertypes "github.com/gruntwork-io/terragrunt/internal/runner/types"
	"github.com/gruntwork-io/terragrunt/tf"
)

// toTFOptions converts RunnerOptions to tf.TFOptions.
// This helper avoids circular dependencies by keeping the conversion
// in the runner/run package which can import both runner/types and tf.
func toTFOptions(runnerOpts *runnertypes.RunnerOptions) *tf.TFOptions {
	if runnerOpts == nil {
		return nil
	}

	return &tf.TFOptions{
		// Binary configuration
		TFPath:                  runnerOpts.TFPath,
		TFPathExplicitlySet:     runnerOpts.TFPathExplicitlySet,
		TerraformImplementation: runnerOpts.TerraformImplementation,
		TerraformCliArgs:        runnerOpts.TerraformCliArgs,

		// Paths
		WorkingDir:           runnerOpts.WorkingDir,
		TerragruntConfigPath: runnerOpts.TerragruntConfigPath,
		DownloadDir:          runnerOpts.DownloadDir,

		// I/O
		Writer:    runnerOpts.Writer,
		ErrWriter: runnerOpts.ErrWriter,

		// Environment
		Env: runnerOpts.Env,

		// Behavior flags
		ForwardTFStdout:        runnerOpts.ForwardTFStdout,
		JSONLogFormat:          runnerOpts.JSONLogFormat,
		Headless:               runnerOpts.Headless,
		LogDisableErrorSummary: runnerOpts.LogDisableErrorSummary,

		// Engine support
		Engine:        runnerOpts.Engine,
		EngineEnabled: runnerOpts.EngineEnabled,

		// Telemetry
		Telemetry: runnerOpts.Telemetry,
	}
}
