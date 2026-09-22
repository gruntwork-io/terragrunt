// Package print implements the 'terragrunt info print' command that outputs Terragrunt context
// information in a structured JSON format. This includes configuration paths, working directories,
// IAM roles, and other essential Terragrunt runtime information useful for debugging and
// automation purposes.
package print

import (
	"context"
	"encoding/json"
	"fmt"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/prepare"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// jsonLayout is how one unit's info is written.
type jsonLayout int

const (
	// prettyJSON indents a unit's info over several lines.
	prettyJSON jsonLayout = iota
	// lineJSON writes a unit's info on one line, so a run over several units
	// reads as JSON Lines and a consumer can parse it a line at a time.
	lineJSON
)

func Run(ctx context.Context, l log.Logger, v *venv.Venv, opts *options.TerragruntOptions) error {
	// If --all flag is set, use discovery to find all units and print info for each one
	if opts.RunAll {
		return runAll(ctx, l, v, opts)
	}

	return runPrint(ctx, l, v, opts, prettyJSON)
}

func runPrint(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	layout jsonLayout,
) error {
	prepared, err := prepare.PrepareConfig(ctx, l, v, opts)
	if err != nil {
		// Even on error, try to print what info we have
		l.Debugf("Fetching info with error: %v", err)

		if printErr := printTerragruntContext(l, v, opts, layout); printErr != nil {
			l.Errorf("Error printing info: %v", printErr)
		}

		return nil
	}

	// Download source
	updatedOpts, err := prepare.PrepareSource(
		ctx,
		l,
		v,
		prepared.Opts,
		prepared.Cfg,
		report.NewReport(),
	)
	if err != nil {
		// Even on error, try to print what info we have
		l.Debugf("Fetching info with error: %v", err)

		if printErr := printTerragruntContext(l, v, opts, layout); printErr != nil {
			l.Errorf("Error printing info: %v", printErr)
		}

		return nil
	}

	return printTerragruntContext(l, v, updatedOpts, layout)
}

func runAll(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
) error {
	d := discovery.NewDiscovery(opts.WorkingDir)

	components, err := d.Discover(ctx, l, v, opts)
	if err != nil {
		return err
	}

	units := components.Filter(component.UnitKind).Sort()

	var errs []error

	for _, c := range units {
		unit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		// The options a unit runs under come from the runner, so the context
		// printed here is the one `run --all` gives that unit, down to the
		// download directory it caches into.
		unitOpts, unitLogger, err := runner.BuildUnitOpts(l, opts, unit)
		if err != nil {
			return err
		}

		// Preparation writes obtained credentials into the env, so each
		// unit gets its own clone to keep them from leaking to siblings.
		unitV := v.WithEnvCloned()
		if err := runPrint(ctx, unitLogger, unitV, unitOpts, lineJSON); err != nil {
			if opts.FailFast {
				return err
			}

			l.Errorf("Print failed: %v", err)

			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
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

func printTerragruntContext(
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	layout jsonLayout,
) error {
	group := InfoOutput{
		ConfigPath:       opts.TerragruntConfigPath,
		DownloadDir:      opts.DownloadDir,
		IAMRole:          opts.IAMRoleOptions.RoleARN,
		TerraformBinary:  opts.TFPath,
		TerraformCommand: opts.TerraformCommand,
		WorkingDir:       opts.WorkingDir,
	}

	marshal := json.Marshal
	if layout == prettyJSON {
		marshal = func(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }
	}

	b, err := marshal(group)
	if err != nil {
		l.Errorf("JSON error marshalling info")
		return err
	}

	if _, err := fmt.Fprintf(v.Writers.Writer, "%s\n", b); err != nil {
		return err
	}

	return nil
}
