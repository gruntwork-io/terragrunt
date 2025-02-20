package config

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	stackDir = ".terragrunt-stack"
)

// StackConfigFile represents the structure of terragrunt.stack.hcl stack file.
type StackConfigFile struct {
	Locals *terragruntLocal `hcl:"locals,block"`
	Units  []*Unit          `hcl:"unit,block"`
}

// Unit represent unit from stack file.
type Unit struct {
	Name   string     `hcl:",label"`
	Source string     `hcl:"source,attr"`
	Path   string     `hcl:"path,attr"`
	Values *cty.Value `hcl:"values,attr"`
}

// ReadOutputs reads the outputs from the unit.
func (u *Unit) ReadOutputs(ctx context.Context, opts *options.TerragruntOptions) (map[string]cty.Value, error) {
	baseDir := filepath.Join(opts.WorkingDir, stackDir)
	unitPath := filepath.Join(baseDir, u.Path)
	configPath := filepath.Join(unitPath, DefaultTerragruntConfigPath)
	opts.Logger.Debugf("Getting output from unit %s in %s", u.Name, unitPath)

	parserCtx := NewParsingContext(ctx, opts)

	jsonBytes, err := getOutputJSONWithCaching(parserCtx, configPath) //nolint: contextcheck

	if err != nil {
		return nil, errors.New(err)
	}

	outputMap, err := TerraformOutputJSONToCtyValueMap(configPath, jsonBytes)

	if err != nil {
		return nil, errors.New(err)
	}

	return outputMap, nil
}

// ReadStackConfigFile reads the terragrunt.stack.hcl file.
func ReadStackConfigFile(ctx context.Context, opts *options.TerragruntOptions) (*StackConfigFile, error) {
	opts.Logger.Debugf("Reading Terragrunt stack config file at %s", opts.TerragruntStackConfigPath)

	parser := NewParsingContext(ctx, opts)

	file, err := hclparse.NewParser(parser.ParserOptions...).ParseFromFile(opts.TerragruntStackConfigPath)
	if err != nil {
		return nil, errors.New(err)
	}
	//nolint:contextcheck
	if err := processLocals(parser, opts, file); err != nil {
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

	if err := ValidateStackConfig(config); err != nil {
		return nil, errors.New(err)
	}

	return config, nil
}

// ValidateStackConfig validates a StackConfigFile instance according to the rules:
// - Unit name, source, and path shouldn't be empty
// - Unit names should be unique
// - Units shouldn't have duplicate paths
func ValidateStackConfig(config *StackConfigFile) error {
	if len(config.Units) == 0 {
		return errors.New("stack config must contain at least one unit")
	}

	validationErrors := &errors.MultiError{}

	names := make(map[string]bool, len(config.Units))
	paths := make(map[string]bool, len(config.Units))

	for i, unit := range config.Units {
		name := strings.TrimSpace(unit.Name)
		path := strings.TrimSpace(unit.Path)

		if name == "" {
			validationErrors = validationErrors.Append(errors.Errorf("unit at index %d has empty name", i))
		}

		if strings.TrimSpace(unit.Source) == "" {
			validationErrors = validationErrors.Append(errors.Errorf("unit '%s' has empty source", unit.Name))
		}

		if path == "" {
			validationErrors = validationErrors.Append(errors.Errorf("unit '%s' has empty path", unit.Name))
		}

		if names[name] {
			validationErrors = validationErrors.Append(errors.Errorf("duplicate unit name found: '%s'", unit.Name))
		}

		if name != "" {
			// save non-empty names for reuse
			names[name] = true
		}

		if paths[path] {
			validationErrors = validationErrors.Append(errors.Errorf("duplicate unit path found: '%s'", unit.Path))
		}

		if path != "" {
			paths[path] = true
		}
	}

	return validationErrors.ErrorOrNil()
}

func processLocals(parser *ParsingContext, opts *options.TerragruntOptions, file *hclparse.File) error {
	localsBlock, err := file.Blocks(MetadataLocals, false)

	if err != nil {
		return errors.New(err)
	}

	if len(localsBlock) == 0 {
		return nil
	}

	if len(localsBlock) > 1 {
		return errors.New(fmt.Sprintf("up to one locals block is allowed per stack file, but found %d in %s", len(localsBlock), file.ConfigPath))
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
		attrs, evaluatedLocals, evaluated, err = attemptEvaluateLocals(
			parser,
			file,
			attrs,
			evaluatedLocals,
		)

		if err != nil {
			opts.Logger.Debugf("Encountered error while evaluating locals in file %s", opts.TerragruntStackConfigPath)

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
