package stack

import (
	"context"
	"path/filepath"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/config"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

func generateOutput(ctx context.Context, opts *options.TerragruntOptions) (map[string]map[string]cty.Value, error) {
	opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, defaultStackFile)
	opts.Logger.Debugf("Generating output from %s", opts.TerragruntStackConfigPath)
	stackFile, err := config.ReadStackConfigFile(ctx, opts)
	if err != nil {
		return nil, errors.New(err)
	}
	unitOutputs := make(map[string]map[string]cty.Value)
	// process each unit and get outputs
	for _, unit := range stackFile.Units {
		opts.Logger.Debugf("Processing unit %s", unit.Name)
		output, err := unit.ReadOutputs(ctx, opts)
		if err != nil {
			return nil, errors.New(err)
		}
		unitOutputs[unit.Name] = output
	}

	return unitOutputs, nil
}
