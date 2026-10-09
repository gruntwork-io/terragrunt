package config

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/topo"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// EvaluateLocalsBlock is a routine to evaluate the locals block in a way to allow references to other locals. This
// will:
//   - Extract a reference to the locals block from the parsed file
//   - Evaluate each local once, after every local it references has been evaluated
//
// This returns a map of the local names to the evaluated expressions (represented as `cty.Value` objects). This will
// error if there are remaining unevaluated locals after all references that can be evaluated has been evaluated.
func EvaluateLocalsBlock(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	file *hclparse.File,
) (map[string]cty.Value, error) {
	localsBlock, err := file.Blocks(MetadataLocals, false)
	if err != nil {
		return nil, err
	}

	if len(localsBlock) == 0 {
		// No locals block referenced in the file
		l.Debugf("Did not find any locals block: skipping evaluation.")
		return nil, nil
	}

	l.Debugf("Found locals block: evaluating the expressions.")

	attrs, err := localsBlock[0].JustAttributes()
	if err != nil {
		l.Debugf("Encountered error while decoding locals block into name expression pairs.")
		return nil, err
	}

	evaluatedLocals, err := evaluateLocalsInOrder(ctx, l, v, pctx, file, attrs)
	if err != nil {
		l.Debugf(
			"Encountered error while evaluating locals in file %s",
			util.RelPathForLog(
				pctx.RootWorkingDir,
				pctx.TerragruntConfigPath,
				pctx.LogShowAbsPaths,
			),
		)

		return evaluatedLocals, err
	}

	if len(evaluatedLocals) < len(attrs) {
		// This is an error because we couldn't evaluate all locals
		l.Debugf("Not all locals could be evaluated:")

		var errs []error

		for _, attr := range attrs {
			if _, ok := evaluatedLocals[attr.Name]; ok {
				continue
			}

			diags := canEvaluateLocals(attr.Expr, evaluatedLocals)
			if err := file.HandleDiagnostics(diags); err != nil {
				errs = append(errs, err)
			}
		}

		if err := errors.Join(errs...); err != nil {
			return nil, CouldNotEvaluateAllLocalsError{Err: err}
		}
	}

	return evaluatedLocals, nil
}

// evaluateLocalsInOrder evaluates attrs in layers of a topological sort over their references to one another. A layer
// holds every attribute whose referenced locals were all evaluated in earlier layers, and each layer evaluates against
// the locals of the layers before it.
//
// An attribute that can never be evaluated is left out of the result, along with every attribute that references it.
// That covers cycles, references to locals the block does not define, and references to variables other than
// `local`, `include`, `feature` and `values`.
//
// Returns the locals evaluated so far and the joined errors of the first layer in which an attribute fails to
// evaluate.
func evaluateLocalsInOrder(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	file *hclparse.File,
	attrs hclparse.Attributes,
) (map[string]cty.Value, error) {
	graph, byName := localsGraph(attrs)
	ready := graph.Roots()
	evaluatedLocals := make(map[string]cty.Value, len(attrs))

	if len(ready) == 0 {
		return evaluatedLocals, nil
	}

	evalCtx, err := CreateTerragruntEvalContext(ctx, l, v, pctx, file.ConfigPath)
	if err != nil {
		l.Errorf(
			"Could not convert include to the execution ctx to evaluate additional locals in file %s",
			file.ConfigPath,
		)

		return evaluatedLocals, err
	}

	for len(ready) > 0 {
		localsAsCtyVal, err := ConvertValuesMapToCtyVal(evaluatedLocals)
		if err != nil {
			l.Errorf(
				"Could not convert evaluated locals to the execution ctx to evaluate additional locals in file %s",
				file.ConfigPath,
			)

			return evaluatedLocals, err
		}

		pctx.Locals = &localsAsCtyVal
		evalCtx.Variables[MetadataLocal] = localsAsCtyVal

		var (
			next                     []string
			newlyEvaluatedLocalNames []string
			errs                     []error
		)

		for _, name := range ready {
			evaluatedVal, err := byName[name].Value(evalCtx)
			if err != nil {
				errs = append(errs, err)
				continue
			}

			evaluatedLocals[name] = evaluatedVal
			newlyEvaluatedLocalNames = append(newlyEvaluatedLocalNames, name)
			next = append(next, graph.Done(name)...)
		}

		l.Debugf(
			"Evaluated %d locals (remaining %d): %s",
			len(newlyEvaluatedLocalNames),
			len(attrs)-len(evaluatedLocals),
			strings.Join(newlyEvaluatedLocalNames, ", "),
		)

		if err := errors.Join(errs...); err != nil {
			return evaluatedLocals, err
		}

		ready = next
	}

	return evaluatedLocals, nil
}

// localsGraph returns a graph over the names of attrs, in which each local waits on the locals it references, along
// with attrs indexed by name. A local that can never be evaluated is left out of the graph, so neither it nor any
// local waiting on it becomes ready.
func localsGraph(attrs hclparse.Attributes) (*topo.Graph[string], map[string]*hclparse.Attribute) {
	byName := make(map[string]*hclparse.Attribute, len(attrs))
	for _, attr := range attrs {
		byName[attr.Name] = attr
	}

	graph := topo.New[string](len(attrs))

	for _, attr := range attrs {
		deps, ok := localDependencies(attr.Expr, byName)
		if !ok {
			continue
		}

		graph.Add(attr.Name, slices.Sorted(maps.Keys(deps))...)
	}

	return graph, byName
}

// localDependencies returns the set of locals in attrs that expression references.
//
// Returns false when expression can never be evaluated inside the locals block, because it references a local absent
// from attrs or a variable that locals cannot see.
func localDependencies(
	expression hcl.Expression,
	attrs map[string]*hclparse.Attribute,
) (map[string]struct{}, bool) {
	var deps map[string]struct{}

	for _, localVar := range expression.Variables() {
		localName, detail := localReference(localVar)
		if detail != "" {
			return nil, false
		}

		if localName == "" {
			continue
		}

		if _, ok := attrs[localName]; !ok {
			return nil, false
		}

		if deps == nil {
			deps = map[string]struct{}{}
		}

		deps[localName] = struct{}{}
	}

	return deps, true
}

// canEvaluateLocals determines if the local expression can be evaluated. An expression can be evaluated if one of the
// following is true:
// - It has no references to other locals.
// - It has references to other locals that have already been evaluated.
// The returned diagnostics carry a human friendly reason for why the expression cannot be evaluated, and are useful
// for error reporting.
func canEvaluateLocals(
	expression hcl.Expression,
	evaluatedLocals map[string]cty.Value,
) hcl.Diagnostics {
	var diags hcl.Diagnostics

	for _, localVar := range expression.Variables() {
		localName, detail := localReference(localVar)

		if _, hasEvaluated := evaluatedLocals[localName]; detail == "" && localName != "" && !hasEvaluated {
			detail = fmt.Sprintf(
				"The local %q could not be evaluated. It is either undefined, part of a reference cycle, "+
					"or depends on a value that locals cannot reference.",
				localName,
			)
		}

		if detail != "" {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Can't evaluate expression",
				Detail:   detail,
				Subject:  expression.Range().Ptr(),
			})
		}
	}

	return diags
}

// localsRoots lists the variables a locals block can reference.
var localsRoots = []string{MetadataLocal, MetadataInclude, MetadataFeatureFlag, MetadataValues}

// localReference classifies a variable referenced from a locals block. It returns the name of the local the reference
// reads, or an empty name when the reference reads a variable locals can see without other locals. The detail is
// non-empty when a locals block can never resolve the reference, and says why.
func localReference(localVar hcl.Traversal) (localName, detail string) {
	rootName := localVar.RootName()

	if !slices.Contains(localsRoots, rootName) {
		return "", fmt.Sprintf(
			"Locals can only reference these variables: %s. This expression references %q.",
			strings.Join(localsRoots, ", "),
			rootName,
		)
	}

	if rootName != MetadataLocal {
		return "", ""
	}

	localName = getLocalName(localVar)
	if localName == "" {
		return "", "The name of the referenced local cannot be determined."
	}

	return localName, ""
}

// getLocalName takes a variable reference encoded as a HCL tree traversal that is rooted at the name `local` and
// returns the underlying variable lookup on the local map. If it is not a local name lookup, this will return empty
// string.
func getLocalName(traversal hcl.Traversal) string {
	split := traversal.SimpleSplit()
	for _, relRaw := range split.Rel {
		switch rel := relRaw.(type) {
		case hcl.TraverseAttr:
			return rel.Name
		default:
			// This means that it is either an operation directly on the locals block, or is an unsupported action (e.g
			// a splat or lookup). Either way, there is no local name.
			continue
		}
	}

	return ""
}

// ------------------------------------------------
// Custom Errors Returned by Functions in this Code
// ------------------------------------------------

type CouldNotEvaluateAllLocalsError struct {
	Err error
}

func (err CouldNotEvaluateAllLocalsError) Error() string {
	return "Could not evaluate all locals in block."
}

func (err CouldNotEvaluateAllLocalsError) Unwrap() error {
	return err.Err
}
