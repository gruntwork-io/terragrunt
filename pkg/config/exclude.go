package config

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// bool values to be used as booleans.
var boolFlagValues = []string{"if", "exclude_dependencies", "no_run"}

// ExcludeConfig configurations for hcl files.
type ExcludeConfig struct {
	ExcludeDependencies *bool    `cty:"exclude_dependencies" hcl:"exclude_dependencies,attr" json:"exclude_dependencies"`
	NoRun               *bool    `cty:"no_run"               hcl:"no_run,attr"               json:"no_run"`
	Actions             []string `cty:"actions"              hcl:"actions,attr"              json:"actions"`
	If                  bool     `cty:"if"                   hcl:"if,attr"                   json:"if"`
}

// IsActionListed checks if the action is listed in the exclude block.
func (e *ExcludeConfig) IsActionListed(action string) bool {
	return runcfg.IsActionListedInExclude(e.Actions, action)
}

// ShouldPreventRun checks if the unit should be prevented from running based on the no_run attribute and current action.
func (e *ExcludeConfig) ShouldPreventRun(action string) bool {
	return runcfg.ShouldPreventRunBasedOnExclude(e.Actions, e.NoRun, e.If, action)
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
func evaluateExcludeBlocks(
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	file *hclparse.File,
) (*ExcludeConfig, error) {
	excludeBlock, err := file.Blocks(MetadataExclude, false)
	if err != nil {
		return nil, err
	}

	if len(excludeBlock) == 0 {
		return nil, nil
	}

	if len(excludeBlock) > 1 {
		// only one block allowed
		return nil, fmt.Errorf(
			"only one %s block is allowed found multiple in %s",
			MetadataExclude,
			file.ConfigPath,
		)
	}

	if err := validateExcludeDependencyReferences(ctx, pctx, l, file); err != nil {
		return nil, err
	}

	attrs, err := excludeBlock[0].JustAttributes()
	if err != nil {
		l.Debugf("Encountered error while decoding exclude block.")
		return nil, err
	}

	evalCtx, err := createTerragruntEvalContext(ctx, pctx, l, file.ConfigPath)
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
			// Null, unknown and sensitive strings skip parsing and reach the checks below.
			if value.Type() == cty.String && !value.IsMarked() && value.IsKnown() && !value.IsNull() {
				val, err := strconv.ParseBool(value.AsString())
				if err != nil {
					return nil, err
				}

				evaluatedAttrs[boolFlag] = cty.BoolVal(val)
			}
		}
	}

	excludeAsCtyVal, err := ConvertValuesMapToCtyVal(evaluatedAttrs)
	if err != nil {
		return nil, err
	}

	if !excludeAsCtyVal.IsWhollyKnown() {
		l.Warnf(
			"Ignoring the exclude block in %s because it reads values that are not known during discovery, such as dependency outputs.",
			file.ConfigPath,
		)

		return nil, nil
	}

	excludeConfig := &ExcludeConfig{}
	if err := CtyToStruct(excludeAsCtyVal, excludeConfig); err != nil {
		return nil, InvalidExcludeBlockError{Err: err, ConfigPath: file.ConfigPath}
	}

	return excludeConfig, nil
}

// validateExcludeDependencyReferences warns about an exclude block that reads a dependency, or rejects it under the exclude-dependency-outputs strict control.
func validateExcludeDependencyReferences(
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	file *hclparse.File,
) error {
	attribute, err := excludeDependencyAttribute(l, file)
	if err != nil {
		return err
	}

	if attribute == "" {
		return nil
	}

	excludeControls := pctx.StrictControls.FilterByNames(controls.ExcludeDependencyOutputs)
	if len(excludeControls.FilterByEnabled()) > 0 {
		return ExcludeReferencesDependencyError{ConfigPath: file.ConfigPath, Attribute: attribute}
	}

	return excludeControls.Evaluate(log.ContextWithLogger(ctx, l))
}

// excludeDependencyAttribute returns the first attribute that reads a dependency in the file's only exclude block, leaving malformed blocks to the parse.
func excludeDependencyAttribute(l log.Logger, file *hclparse.File) (string, error) {
	excludeBlocks, err := file.Blocks(MetadataExclude, true)
	if err != nil {
		return "", err
	}

	if len(excludeBlocks) != 1 {
		return "", nil
	}

	attrs, diags := excludeBlocks[0].Body.JustAttributes()
	names := slices.Sorted(maps.Keys(attrs))

	invalidName := slices.ContainsFunc(names, func(name string) bool {
		return !hclsyntax.ValidIdentifier(name)
	})

	if diags.HasErrors() || invalidName {
		l.Debugf("Skipping the dependency check for the malformed exclude block in %s; parsing reports the error", file.ConfigPath)

		return "", nil
	}

	for _, name := range names {
		if expressionReferencesDependency(attrs[name].Expr) {
			return name, nil
		}
	}

	return "", nil
}
