package stack

import (
	"context"
	"path/filepath"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/config"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

func generateOutput(ctx context.Context, opts *options.TerragruntOptions) error {
	opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, defaultStackFile)
	opts.Logger.Infof("Generating output from %s", opts.TerragruntStackConfigPath)
	stackFile, err := config.ReadStackConfigFile(ctx, opts)

	if err != nil {
		return errors.New(err)
	}

	var unitOutputs map[string]map[string]cty.Value

	// process each unit and get outputs
	for _, unit := range stackFile.Units {
		opts.Logger.Debugf("Processing unit %s", unit.Name)
		// get the output from the unit
		output, err := getOutput(ctx, opts, unit)
		if err != nil {
			return errors.New(err)
		}
		unitOutputs[unit.Name] = output
	}

	return nil
}

func getOutput(ctx context.Context, opts *options.TerragruntOptions, unit *config.Unit) (map[string]cty.Value, error) {

	return nil, nil
}
