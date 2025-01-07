package config

import (
	"context"

	"github.com/zclconf/go-cty/cty"

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

	if err := processLocals(terragruntOptions, parseCtx, file); err != nil {
		return nil, errors.New(err)
	}

	evalParsingContext, err := createTerragruntEvalContext(parseCtx, file.ConfigPath)
	config := &StackConfigFile{}
	if err := file.Decode(config, evalParsingContext); err != nil {
		return nil, err
	}

	return config, nil
}

func processLocals(terragruntOptions *options.TerragruntOptions, parseCtx *ParsingContext, file *hclparse.File) error {
	localsBlock, err := file.Blocks(MetadataLocals, false)
	if err != nil {
		return errors.New(err)
	}
	if len(localsBlock) == 0 {
		return nil
	}
	attrs, err := localsBlock[0].JustAttributes()
	if err != nil {
		return errors.New(err)
	}
	evaluatedLocals := map[string]cty.Value{}
	evaluated := true

	for iterations := 0; len(attrs) > 0 && evaluated; iterations++ {
		if iterations > MaxIter {
			// Reached maximum supported iterations, which is most likely an infinite loop bug so cut the iteration
			// short an return an error.
			return errors.New(MaxIterError{})
		}

		var err error
		attrs, evaluatedLocals, evaluated, err = attemptEvaluateLocals(
			parseCtx,
			file,
			attrs,
			evaluatedLocals,
		)

		if err != nil {
			terragruntOptions.Logger.Debugf("Encountered error while evaluating locals in file %s", terragruntOptions.TerragrungStackConfigPath)
			return errors.New(err)
		}
	}
	localsAsCtyVal, err := convertValuesMapToCtyVal(evaluatedLocals)
	if err != nil {
		return errors.New(err)
	}
	parseCtx.Locals = &localsAsCtyVal
	return nil
}
