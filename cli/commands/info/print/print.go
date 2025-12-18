// Package print implements the 'terragrunt info print' command that outputs Terragrunt context
// information in a structured JSON format. This includes configuration paths, working directories,
// IAM roles, and other essential Terragrunt runtime information useful for debugging and
// automation purposes.
package print

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	prepared, err := run.PrepareConfig(ctx, l, opts)
	if err != nil {
		// Even on error, try to print what info we have
		l.Debugf("Fetching info with error: %v", err)

		if printErr := printTerragruntContext(l, opts); printErr != nil {
			l.Errorf("Error printing info: %v", printErr)
		}

		return nil
	}

	// Download source
	updatedOpts, err := run.PrepareSource(ctx, prepared.Logger, prepared.UpdatedOpts, prepared.TerragruntConfig, report.NewReport())
	if err != nil {
		// Even on error, try to print what info we have
		l.Debugf("Fetching info with error: %v", err)

		if printErr := printTerragruntContext(l, opts); printErr != nil {
			l.Errorf("Error printing info: %v", printErr)
		}

		return nil
	}

	return printTerragruntContext(prepared.Logger, updatedOpts)
}

// InfoOutput represents the structured output of the info command
type InfoOutput struct {
	ConfigPath       string `json:"config_path"`
	DownloadDir      string `json:"download_dir"`
	IAMRole          string `json:"iam_role"`
	TerraformBinary  string `json:"terraform_binary"`
	TerraformCommand string `json:"terraform_command"`
	WorkingDir       string `json:"working_dir"`
}

func printTerragruntContext(l log.Logger, opts *options.TerragruntOptions) error {
	group := InfoOutput{
		ConfigPath:       opts.TerragruntConfigPath,
		DownloadDir:      opts.DownloadDir,
		IAMRole:          opts.IAMRoleOptions.RoleARN,
		TerraformBinary:  opts.TFPath,
		TerraformCommand: opts.TerraformCommand,
		WorkingDir:       opts.WorkingDir,
	}

	b, err := json.MarshalIndent(group, "", "  ")
	if err != nil {
		l.Errorf("JSON error marshalling info")
		return errors.New(err)
	}

	if _, err := fmt.Fprintf(opts.Writer, "%s\n", b); err != nil {
		return errors.New(err)
	}

	return nil
}
