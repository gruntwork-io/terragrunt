package config

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
)

// MaxIter is the maximum number of depth we support in recursively evaluating locals.
const MaxIter = 1000

// evaluateLocalsBlock is a routine to evaluate the locals block in a way to allow references to other locals. This
// will:
//   - Extract a reference to the locals block from the parsed file
//   - Continuously evaluate the block until all references are evaluated, defering evaluation of anything that references
//     other locals until those references are evaluated.
//
// This returns a map of the local names to the evaluated expressions (represented as `cty.Value` objects). This will
// error if there are remaining unevaluated locals after all references that can be evaluated has been evaluated.
func evaluateLocalsBlock(ctx *ParsingContext, file *hclparse.File) (map[string]cty.Value, error) {
	localsBlock, err := file.Blocks(MetadataLocals, false)
	if err != nil {
		return nil, err
	}
	if len(localsBlock) == 0 {
		// No locals block referenced in the file
		ctx.TerragruntOptions.Logger.Debugf("Did not find any locals block: skipping evaluation.")
		return nil, nil
	}

	ctx.TerragruntOptions.Logger.Debugf("Found locals block: evaluating the expressions.")

	attrs, err := localsBlock[0].JustAttributes()
	if err != nil {
		ctx.TerragruntOptions.Logger.Debugf("Encountered error while decoding locals block into name expression pairs.")
		return nil, err
	}

	// Continuously attempt to evaluate the locals until there are no more locals to evaluate, or we can't evaluate
	// further.
	evaluatedLocals := map[string]cty.Value{}
	evaluated := true
	for iterations := 0; len(attrs) > 0 && evaluated; iterations++ {
		if iterations > MaxIter {
			// Reached maximum supported iterations, which is most likely an infinite loop bug so cut the iteration
			// short an return an error.
			return nil, errors.WithStackTrace(MaxIterError{})
		}

		var err error
		attrs, evaluatedLocals, evaluated, err = attemptEvaluateLocals(
			ctx,
			file,
			attrs,
			evaluatedLocals,
		)
		if err != nil {
			ctx.TerragruntOptions.Logger.Debugf("Encountered error while evaluating locals in file %s", file.ConfigPath)
			return nil, err
		}
	}

	if len(attrs) > 0 {
		// This is an error because we couldn't evaluate all locals
		ctx.TerragruntOptions.Logger.Errorf("Not all locals could be evaluated:")
		for _, local := range attrs {
			_, reason := canEvaluateLocals(local.Expr, evaluatedLocals)
			ctx.TerragruntOptions.Logger.Errorf("\t- %s [REASON: %s]", local.Name, reason)
		}
		return nil, errors.WithStackTrace(CouldNotEvaluateAllLocalsError{})
	}

	return evaluatedLocals, nil
}

// attemptEvaluateLocals attempts to evaluate the locals block given the map of already evaluated locals, replacing
// references to locals with the previously evaluated values. This will return:
// - the list of remaining locals that were unevaluated in this attempt
// - the updated map of evaluated locals after this attempt
// - whether or not any locals were evaluated in this attempt
// - any errors from the evaluation
func attemptEvaluateLocals(
	ctx *ParsingContext,
	file *hclparse.File,
	attrs hclparse.Attributes,
	evaluatedLocals map[string]cty.Value,
) (unevaluatedAttrs hclparse.Attributes, newEvaluatedLocals map[string]cty.Value, evaluated bool, err error) {
	localsAsCtyVal, err := convertValuesMapToCtyVal(evaluatedLocals)
	if err != nil {
		ctx.TerragruntOptions.Logger.Errorf("Could not convert evaluated locals to the execution ctx to evaluate additional locals in file %s", file.ConfigPath)
		return nil, evaluatedLocals, false, err
	}
	ctx.Locals = &localsAsCtyVal

	evalCtx, err := createTerragruntEvalContext(ctx, file.ConfigPath)
	if err != nil {
		ctx.TerragruntOptions.Logger.Errorf("Could not convert include to the execution ctx to evaluate additional locals in file %s", file.ConfigPath)
		return nil, evaluatedLocals, false, err
	}

	// Track the locals that were evaluated for logging purposes
	newlyEvaluatedLocalNames := []string{}

	unevaluatedAttrs = hclparse.Attributes{}
	evaluated = false
	newEvaluatedLocals = map[string]cty.Value{}
	for key, val := range evaluatedLocals {
		newEvaluatedLocals[key] = val
	}
	for _, attr := range attrs {
		localEvaluated, _ := canEvaluateLocals(attr.Expr, evaluatedLocals)
		if localEvaluated {
			evaluatedVal, err := attr.Value(evalCtx)
			if err != nil {
				return nil, evaluatedLocals, false, err
			}
			newEvaluatedLocals[attr.Name] = evaluatedVal
			newlyEvaluatedLocalNames = append(newlyEvaluatedLocalNames, attr.Name)
			evaluated = true
		} else {
			unevaluatedAttrs = append(unevaluatedAttrs, attr)
		}
	}

	ctx.TerragruntOptions.Logger.Debugf(
		"Evaluated %d locals (remaining %d): %s",
		len(newlyEvaluatedLocalNames),
		len(unevaluatedAttrs),
		strings.Join(newlyEvaluatedLocalNames, ", "),
	)
	return unevaluatedAttrs, newEvaluatedLocals, evaluated, nil
}

// canEvaluateLocals determines if the local expression can be evaluated. An expression can be evaluated if one of the
// following is true:
// - It has no references to other locals.
// - It has references to other locals that have already been evaluated.
// Note that the second return value is a human friendly reason for why the expression can not be evaluated, and is
// useful for error reporting.
func canEvaluateLocals(expression hcl.Expression, evaluatedLocals map[string]cty.Value) (bool, string) {
	vars := expression.Variables()
	if len(vars) == 0 {
		// If there are no local variable references, we can evaluate this expression.
		return true, ""
	}

	for _, var_ := range vars {
		// This should never happen, but if it does, we can't evaluate this expression.
		if var_.IsRelative() {
			reason := "You've reached an impossible condition and is almost certainly a bug in terragrunt. Please open an issue at github.com/gruntwork-io/terragrunt with this message and the contents of your terragrunt.hcl file that caused this."
			return false, reason
		}

		rootName := var_.RootName()

		// If the variable is `include`, then we can evaluate it now
		if rootName == MetadataInclude {
			continue
		}

		// We can't evaluate any variable other than `local`
		if rootName != "local" {
			reason := fmt.Sprintf(
				"Can't evaluate expression at %s: you can only reference other local variables here, but it looks like you're referencing something else (%s is not defined)",
				expression.Range(),
				rootName,
			)
			return false, reason
		}

		// If we can't get any local name, we can't evaluate it.
		localName := getLocalName(var_)
		if localName == "" {
			reason := fmt.Sprintf(
				"Can't evaluate expression at %s because local var name can not be determined.",
				expression.Range(),
			)
			return false, reason
		}

		// If the referenced local isn't evaluated, we can't evaluate this expression.
		_, hasEvaluated := evaluatedLocals[localName]
		if !hasEvaluated {
			reason := fmt.Sprintf(
				"Can't evaluate expression at %s because local reference '%s' is not evaluated. Either it is not ready yet in the current pass, or there was an error evaluating it in an earlier stage.",
				expression.Range(),
				localName,
			)
			return false, reason
		}
	}

	// If we made it this far, this means all the variables referenced are accounted for and we can evaluate this
	// expression.
	return true, ""
}

// getLocalName takes a variable reference encoded as a HCL tree traversal that is rooted at the name `local` and
// returns the underlying variable lookup on the local map. If it is not a local name lookup, this will return empty
// string.
func getLocalName(traversal hcl.Traversal) string {
	if traversal.IsRelative() {
		return ""
	}

	if traversal.RootName() != "local" {
		return ""
	}

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

type CouldNotEvaluateAllLocalsError struct{}

func (err CouldNotEvaluateAllLocalsError) Error() string {
	return "Could not evaluate all locals in block."
}

type MaxIterError struct{}

func (err MaxIterError) Error() string {
	return "Maximum iterations reached in attempting to evaluate locals. This is most likely a bug in Terragrunt. Please file an issue on the project: https://github.com/gruntwork-io/terragrunt/issues"
}
