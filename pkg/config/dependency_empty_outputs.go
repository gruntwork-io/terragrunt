package config

import (
	"errors"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// dependencyOutputsAttr is the attribute of a dependency value that has its outputs.
const dependencyOutputsAttr = "outputs"

// dependencyAddress identifies one dependency value in the `dependency` variable: a whole block, or one
// instance of an expanded block.
type dependencyAddress struct {
	name        string
	instanceKey string
	expanded    bool
}

// withNoOutputsError joins a [TerragruntOutputTargetNoOutputs] error to err when err has a diagnostic for a
// reference to a missing output of a dependency whose outputs resolved to an empty object. It returns err
// unchanged otherwise, and always when the empty-dependency-outputs experiment is disabled.
//
// With the experiment enabled, a dependency that has no outputs resolves to an empty object, so a reference to
// one of its outputs reaches evaluation and fails there with a diagnostic that says nothing about the
// dependency being unapplied.
func withNoOutputsError(v *venv.Venv, pctx *ParsingContext, err error) error {
	if !pctx.Experiments.Evaluate(experiment.EmptyDependencyOutputs) {
		return err
	}

	var diags hcl.Diagnostics
	if !errors.As(err, &diags) {
		return err
	}

	for _, diag := range diags {
		address, ok := missingOutputReference(diag)
		if !ok || !address.hasEmptyOutputs(*pctx.DecodedDependencies) {
			continue
		}

		noOutputs, ok := address.noOutputsError(v, pctx)
		if !ok {
			continue
		}

		return errors.Join(noOutputs, err)
	}

	return err
}

// missingOutputReference returns the address of the dependency in a reference such as
// dependency.vpc.outputs.vpc_id when diag reports the step after `outputs`, which is the output the
// dependency lacks.
func missingOutputReference(diag *hcl.Diagnostic) (dependencyAddress, bool) {
	if diag.Subject == nil {
		return dependencyAddress{}, false
	}

	switch expr := diag.Expression.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		return literalOutputReference(expr.Traversal, *diag.Subject)
	case *hclsyntax.RelativeTraversalExpr:
		return computedOutputReference(expr, diag)
	default:
		return dependencyAddress{}, false
	}
}

// literalOutputReference handles dependency.vpc.outputs.vpc_id and dependency.vpc["key"].outputs.vpc_id,
// which HCL parses as one traversal.
func literalOutputReference(traversal hcl.Traversal, subject hcl.Range) (dependencyAddress, bool) {
	const nameAndOutputSteps = 2

	if traversal.RootName() != MetadataDependency || len(traversal) < nameAndOutputSteps+1 {
		return dependencyAddress{}, false
	}

	name, ok := traversal[1].(hcl.TraverseAttr)
	if !ok {
		return dependencyAddress{}, false
	}

	address := dependencyAddress{name: name.Name}
	rest := traversal[nameAndOutputSteps:]

	if index, ok := rest[0].(hcl.TraverseIndex); ok {
		key, ok := instanceKeyString(index.Key)
		if !ok {
			return dependencyAddress{}, false
		}

		address.instanceKey = key
		address.expanded = true
		rest = rest[1:]
	}

	return address, reportsStepAfterOutputs(rest, subject)
}

// computedOutputReference handles dependency.vpc[local.key].outputs.vpc_id, which HCL parses as an index
// expression followed by a relative traversal.
func computedOutputReference(expr *hclsyntax.RelativeTraversalExpr, diag *hcl.Diagnostic) (dependencyAddress, bool) {
	const dependencyNameSteps = 2

	index, ok := expr.Source.(*hclsyntax.IndexExpr)
	if !ok {
		return dependencyAddress{}, false
	}

	collection, ok := index.Collection.(*hclsyntax.ScopeTraversalExpr)
	if !ok || collection.Traversal.RootName() != MetadataDependency ||
		len(collection.Traversal) != dependencyNameSteps {
		return dependencyAddress{}, false
	}

	name, ok := collection.Traversal[1].(hcl.TraverseAttr)
	if !ok {
		return dependencyAddress{}, false
	}

	keyVal, diags := index.Key.Value(diag.EvalContext)
	if diags.HasErrors() {
		return dependencyAddress{}, false
	}

	key, ok := instanceKeyString(keyVal)
	if !ok {
		return dependencyAddress{}, false
	}

	address := dependencyAddress{name: name.Name, instanceKey: key, expanded: true}

	return address, reportsStepAfterOutputs(expr.Traversal, *diag.Subject)
}

// reportsStepAfterOutputs reports whether steps start with `.outputs` and subject is the step that follows.
func reportsStepAfterOutputs(steps hcl.Traversal, subject hcl.Range) bool {
	const outputsAndOutputSteps = 2

	if len(steps) < outputsAndOutputSteps {
		return false
	}

	outputs, ok := steps[0].(hcl.TraverseAttr)

	return ok && outputs.Name == dependencyOutputsAttr && steps[1].SourceRange() == subject
}

// instanceKeyString returns key the way an expansion instance stores it, where a count index is its decimal
// string.
func instanceKeyString(key cty.Value) (string, bool) {
	key, _ = key.Unmark()

	str, err := convert.Convert(key, cty.String)
	if err != nil || !str.IsKnown() || str.IsNull() {
		return "", false
	}

	return str.AsString(), true
}

// hasEmptyOutputs reports whether the dependency at this address in dependencies, the value of the
// `dependency` variable, has outputs that resolved to an empty object.
func (address dependencyAddress) hasEmptyOutputs(dependencies cty.Value) bool {
	traversal := hcl.Traversal{hcl.TraverseAttr{Name: address.name}}

	if address.expanded {
		traversal = append(traversal, hcl.TraverseIndex{Key: cty.StringVal(address.instanceKey)})
	}

	traversal = append(traversal, hcl.TraverseAttr{Name: dependencyOutputsAttr})

	outputs, diags := traversal.TraverseRel(dependencies)

	return !diags.HasErrors() && outputs.RawEquals(cty.EmptyObjectVal)
}

// noOutputsError returns the [TerragruntOutputTargetNoOutputs] error for the dependency block at this
// address, and false when the config being parsed has no such block.
func (address dependencyAddress) noOutputsError(v *venv.Venv, pctx *ParsingContext) (error, bool) {
	for i := range pctx.DependencyBlocks {
		dep := &pctx.DependencyBlocks[i]

		instanceKey, expanded := dep.instanceKey()
		if dep.Name != address.name || instanceKey != address.instanceKey || expanded != address.expanded {
			continue
		}

		configPath, ok := dep.configPathString()
		if !ok {
			return nil, false
		}

		return TerragruntOutputTargetNoOutputs{
			targetName:    dep.Name,
			targetPath:    configPath,
			targetConfig:  getCleanedTargetConfigPath(v.FS, configPath, pctx.TerragruntConfigPath),
			currentConfig: pctx.TerragruntConfigPath,
		}, true
	}

	return nil, false
}
