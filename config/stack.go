package config

import (
	"context"
	"fmt"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

// StackConfigFile represents the structure of terragrunt.stack.hcl stack file
type StackConfigFile struct {
	Locals *terragruntLocal `hcl:"locals,block" cty:"locals"`
	Units  []*Unit          `hcl:"unit,block"   cty:"unit"`
}

// Unit represent unit from stack file.
type Unit struct {
	Name   string `hcl:",label"      cty:"name"`
	Source string `hcl:"source,attr" cty:"source"`
	Path   string `hcl:"path,attr"   cty:"path"`
}

func ReadStackConfigFile(ctx context.Context, terragruntOptions *options.TerragruntOptions) (*StackConfigFile, error) {
	terragruntOptions.Logger.Debugf("Reading Terragrunt stack config file at %s", terragruntOptions.TerragruntStackConfigPath)

	parser := NewParsingContext(ctx, terragruntOptions)

	file, err := hclparse.NewParser(parser.ParserOptions...).ParseFromFile(terragruntOptions.TerragruntStackConfigPath)
	if err != nil {
		return nil, errors.New(err)
	}
	//nolint:contextcheck
	if err := processLocals(parser, terragruntOptions, file); err != nil {
		return nil, errors.New(err)
	}
	//nolint:contextcheck
	evalParsingContext, err := createTerragruntEvalContext(parser, file.ConfigPath)
	if err != nil {
		return nil, errors.New(err)
	}

	config := &StackConfigFile{}
	if err := file.Decode(config, evalParsingContext); err != nil {
		return nil, errors.New(err)
	}

	return config, nil
}

func processLocals(parser *ParsingContext, terragruntOptions *options.TerragruntOptions, file *hclparse.File) error {
	localsBlock, err := file.Blocks(MetadataLocals, false)

	if err != nil {
		return errors.New(err)
	}

	if len(localsBlock) == 0 {
		return nil
	}

	if len(localsBlock) > 1 {
		return errors.New(fmt.Errorf("only one locals block is allowed in a terragrunt stack file, but found %d in %s", len(localsBlock), file.ConfigPath))
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
			// short and return an error.
			return errors.New(MaxIterError{})
		}

		var err error
		//nolint:contextcheck
		attrs, evaluatedLocals, evaluated, err = attemptEvaluateLocals(
			parser,
			file,
			attrs,
			evaluatedLocals,
		)

		if err != nil {
			terragruntOptions.Logger.Debugf("Encountered error while evaluating locals in file %s", terragruntOptions.TerragruntStackConfigPath)
			return errors.New(err)
		}
	}

	localsAsCtyVal, err := convertValuesMapToCtyVal(evaluatedLocals)

	if err != nil {
		return errors.New(err)
	}

	parser.Locals = &localsAsCtyVal

	return nil
}
