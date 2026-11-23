package validate

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/valuesblock"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const summaryUndeclaredValue = "Undeclared value"

// checkUnitValueReferences collects a diagnostic for each read of values.<name> in the unit config at configPath that
// its values block does not declare.
func (p ComponentParser) checkUnitValueReferences(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *config.Store,
	pctx *config.ParsingContext,
	configPath string,
) error {
	if !pctx.Experiments.Evaluate(experiment.ValuesBlock) {
		return nil
	}

	file, err := syntaxTree(ctx, l, v, s, pctx, configPath)
	if err != nil {
		return err
	}

	return p.checkValueReferences(pctx, file, valuesblock.KindUnit)
}

// checkValueReferences collects a diagnostic for each read of values.<name> in file that its values block does not
// declare, along with the diagnostics of the values block itself. It checks no reads when file has no values block.
//
// Returns [config.BlockNestingExceededError] when the blocks of file nest too deep.
func (p ComponentParser) checkValueReferences(
	pctx *config.ParsingContext,
	file *hclparse.File,
	kind valuesblock.Kind,
) error {
	if !pctx.Experiments.Evaluate(experiment.ValuesBlock) {
		return nil
	}

	decls, declDiags := valuesblock.Decode(file.Body)
	if decls == nil {
		return p.Collector.keepDistinct(file.File, declDiags)
	}

	refs, err := config.ValueReferences(file)
	if err != nil {
		return err
	}

	var diags hcl.Diagnostics

	for _, ref := range refs {
		if _, ok := decls.Lookup(ref.Name); ok {
			continue
		}

		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  summaryUndeclaredValue,
			Detail:   undeclaredValueDetail(kind, &ref, decls),
			Subject:  ref.Range.Ptr(),
		})
	}

	p.Collector.dropEvaluationDiagnostics(diags)

	return p.Collector.keepDistinct(file.File, append(declDiags, diags...))
}

// undeclaredValueDetail explains that decls, the values block of the kind of component, does not declare the value ref
// reads. For a read that try or can guards, it says the guard always falls back, and suggests a default in place of
// the guard.
func undeclaredValueDetail(kind valuesblock.Kind, ref *config.ValueReference, decls *valuesblock.Declarations) string {
	if ref.Guard == "" {
		return fmt.Sprintf("The values block of this %s does not declare %q.", kind, ref.Name) + decls.Hint(ref.Name) +
			fmt.Sprintf("\n\nFix the reference, or declare %q in the values block.", ref.Name)
	}

	result := "returns its fallback"
	if ref.Guard == config.FuncCan {
		result = "returns false"
	}

	return fmt.Sprintf(
		"The values block of this %s does not declare %q, so %s() always %s.",
		kind,
		ref.Name,
		ref.Guard,
		result,
	) + decls.Hint(ref.Name) + fmt.Sprintf(
		"\n\nTo make a value optional, declare it with a default in the values block, and read it without %s().",
		ref.Guard,
	)
}
