package config

import (
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/zclconf/go-cty/cty"
)

// ExcludeConfig configurations for hcl files.
type ExcludeConfig struct {
	If                  bool     `cty:"if" hcl:"if,attr" json:"if"`
	Actions             []string `cty:"actions" hcl:"actions,attr" json:"actions"`
	ExcludeDependencies bool     `cty:"exclude_dependencies" hcl:"exclude_dependencies,attr" json:"exclude_dependencies"`
}

// Clone returns a new instance of ExcludeConfig with the same values as the original.
func (e *ExcludeConfig) Clone() *ExcludeConfig {
	return &ExcludeConfig{
		If:                  e.If,
		Actions:             e.Actions,
		ExcludeDependencies: e.ExcludeDependencies,
	}
}

func (e *ExcludeConfig) Merge(exclude *ExcludeConfig) {
	// copy not empty fields
	e.If = exclude.If
	if len(exclude.Actions) > 0 {
		e.Actions = exclude.Actions
	}
	e.ExcludeDependencies = exclude.ExcludeDependencies
}

// EvaluateExcludeBlocks evaluates the exclude block in the parsed file.
func EvaluateExcludeBlocks(ctx *ParsingContext, file *hclparse.File) (*ExcludeConfig, error) {

	excludeBlock, err := file.Blocks(MetadataExclude, false)
	if err != nil {
		return nil, err
	}

	if len(excludeBlock) == 0 {
		return nil, nil
	}

	if len(excludeBlock) > 1 {
		// only one block allowed
		return nil, errors.Errorf("Only one %s block is allowed found multiple in %s", MetadataExclude, file.ConfigPath)
	}

	attrs, err := excludeBlock[0].JustAttributes()
	if err != nil {
		ctx.TerragruntOptions.Logger.Debugf("Encountered error while decoding exclude block.")
		return nil, err
	}

	evalCtx, err := createTerragruntEvalContext(ctx, file.ConfigPath)
	if err != nil {
		ctx.TerragruntOptions.Logger.Errorf("Failed to create eval context %s", file.ConfigPath)
		return nil, err
	}

	evaluatedAttrs := map[string]cty.Value{}

	for _, attr := range attrs {
		value, err := attr.Value(evalCtx)
		if err != nil {
			ctx.TerragruntOptions.Logger.Debugf("Encountered error while evaluating exclude block in file %s", file.ConfigPath)
			return nil, err
		}
		evaluatedAttrs[attr.Name] = value
	}

	excludeAsCtyVal, err := convertValuesMapToCtyVal(evaluatedAttrs)
	if err != nil {
		return nil, err
	}

	// convert cty map to ExcludeConfig
	excludeConfig := &ExcludeConfig{}
	if err := CtyToStruct(excludeAsCtyVal, excludeConfig); err != nil {
		return nil, err
	}

	return excludeConfig, nil
}
