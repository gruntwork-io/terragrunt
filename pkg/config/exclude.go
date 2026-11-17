package config

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
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

// Excludes reports whether the block excludes its unit from action. A block
// with a false `if` excludes nothing, whatever actions it lists.
func (e *ExcludeConfig) Excludes(action string) bool {
	return e.If && e.IsActionListed(action)
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
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
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

	attrs, err := excludeBlock[0].JustAttributes()
	if err != nil {
		l.Debugf("Encountered error while decoding exclude block.")
		return nil, err
	}

	evalCtx, err := CreateTerragruntEvalContext(ctx, l, v, pctx, file.ConfigPath)
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

	if err := excludeBlock[0].HandleDiagnostics(excludeIfDiagnostics(excludeBlock[0], attrs, evaluatedAttrs)); err != nil {
		return nil, err
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

// excludeIfDiagnostics reports an `if` that block leaves out or that values holds as null, with the diagnostics the
// full parse reports for the same block. An unknown or sensitive `if` reports nothing.
func excludeIfDiagnostics(
	block *hclparse.Block,
	attrs hclparse.Attributes,
	values map[string]cty.Value,
) hcl.Diagnostics {
	const name = "if"

	value, ok := values[name]
	if !ok {
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Missing required argument",
			Detail:   fmt.Sprintf("The argument %q is required, but no definition was found.", name),
			Subject:  block.Body.MissingItemRange().Ptr(),
		}}
	}

	if value.IsMarked() || !value.IsKnown() || !value.IsNull() {
		return nil
	}

	for _, attr := range attrs {
		if attr.Name != name {
			continue
		}

		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Unsuitable value type",
			Detail:   "Unsuitable value: null value is not allowed",
			Subject:  attr.Expr.StartRange().Ptr(),
			Context:  attr.Expr.Range().Ptr(),
		}}
	}

	return nil
}
