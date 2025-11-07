package config

import (
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/zclconf/go-cty/cty"
)

const (
	allActions              = "all"               // handle all actions
	allExcludeOutputActions = "all_except_output" // handle all exclude output actions
	tgOutput                = "output"
)

// bool values to be used as booleans.
var boolFlagValues = []string{"if", "exclude_dependencies", "no_run"}

// ExcludeConfig configurations for hcl files.
type ExcludeConfig struct {
	ExcludeDependencies *bool    `cty:"exclude_dependencies" hcl:"exclude_dependencies,attr" json:"exclude_dependencies"`
	NoRun               *bool    `cty:"no_run" hcl:"no_run,attr" json:"no_run"`
	Actions             []string `cty:"actions" hcl:"actions,attr" json:"actions"`
	If                  bool     `cty:"if" hcl:"if,attr" json:"if"`
}

// IsActionListed checks if the action is listed in the exclude block.
func (e *ExcludeConfig) IsActionListed(action string) bool {
	if len(e.Actions) == 0 {
		return false
	}

	for _, checkAction := range e.Actions {
		if checkAction == allActions { // if actions contains all, return true in all cases
			return true
		}

		if checkAction == allExcludeOutputActions && action != tgOutput {
			return true
		}

		if checkAction == strings.ToLower(action) {
			return true
		}
	}

	return false
}

// ShouldPreventRun checks if the unit should be prevented from running based on the no_run attribute and current action.
func (e *ExcludeConfig) ShouldPreventRun(action string) bool {
	return e.NoRun != nil && *e.NoRun && e.If && e.IsActionListed(action)
}

// Clone returns a new instance of ExcludeConfig with the same values as the original.
func (e *ExcludeConfig) Clone() *ExcludeConfig {
	return &ExcludeConfig{
		If:                  e.If,
		Actions:             e.Actions,
		ExcludeDependencies: e.ExcludeDependencies,
		NoRun:               e.NoRun,
	}
}

// Merge merges the values of the provided ExcludeConfig into the original.
func (e *ExcludeConfig) Merge(exclude *ExcludeConfig) {
	// copy not empty fields
	e.If = exclude.If
	if len(exclude.Actions) > 0 {
		e.Actions = exclude.Actions
	}

	e.ExcludeDependencies = exclude.ExcludeDependencies
	e.NoRun = exclude.NoRun
}

// evaluateExcludeBlocks evaluates the exclude block in the parsed file.
func evaluateExcludeBlocks(ctx *ParsingContext, l log.Logger, file *hclparse.File) (*ExcludeConfig, error) {
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
		l.Debugf("Encountered error while decoding exclude block.")
		return nil, err
	}

	evalCtx, err := createTerragruntEvalContext(ctx, l, file.ConfigPath)
	if err != nil {
		l.Errorf("Failed to create eval context %s", file.ConfigPath)
		return nil, err
	}

	evaluatedAttrs := map[string]cty.Value{}

	for _, attr := range attrs {
		value, err := attr.Value(evalCtx)
		if err != nil {
			l.Debugf("Encountered error while evaluating exclude block in file %s", file.ConfigPath)

			return nil, err
		}

		evaluatedAttrs[attr.Name] = value
	}

	for _, boolFlag := range boolFlagValues {
		if value, ok := evaluatedAttrs[boolFlag]; ok {
			if value.Type() == cty.String { // handle bool flag value
				val, err := strconv.ParseBool(value.AsString())
				if err != nil {
					return nil, errors.New(err)
				}

				evaluatedAttrs[boolFlag] = cty.BoolVal(val)
			}
		}
	}

	excludeAsCtyVal, err := ConvertValuesMapToCtyVal(evaluatedAttrs)
	if err != nil {
		return nil, err
	}

	// convert cty map to ExcludeConfig
	excludeConfig := &ExcludeConfig{}
	if err := CtyToStruct(excludeAsCtyVal, excludeConfig); err != nil {
		return nil, errors.Unwrap(err)
	}

	return excludeConfig, nil
}
