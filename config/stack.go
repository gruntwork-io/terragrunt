package config

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

// StackConfigFile represents the structure of terragrunt.stack.hcl stack file
type StackConfigFile struct {
	Locals *terragruntLocal `cty:"locals"  hcl:"locals,block"`
	Units  []*Unit          `cty:"unit" hcl:"unit,block"`
}

func ReadStackConfigFile(ctx context.Context, terragruntOptions *options.TerragruntOptions) (*StackConfigFile, error) {
	terragruntOptions.Logger.Debugf("Reading Terragrunt stack config file at %s", terragruntOptions.TerragrungStackConfigPath)

	parseCtx := NewParsingContext(ctx, terragruntOptions)

	file, err := hclparse.NewParser(parseCtx.ParserOptions...).ParseFromFile(terragruntOptions.TerragrungStackConfigPath)
	if err != nil {
		return nil, errors.New(err)
	}

	evalParsingContext, err := createTerragruntEvalContext(parseCtx, file.ConfigPath)

	config := &StackConfigFile{}
	if err := file.Decode(config, evalParsingContext); err != nil {
		return nil, err
	}

	return config, nil
}
