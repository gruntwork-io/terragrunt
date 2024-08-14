package config

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
)

// MaxIter is the maximum number of depth we support in recursively evaluating locals.
const MaxIter = 1000

// EvaluateLocalsBlock is a routine to evaluate the locals block in a way to allow references to other locals. This
// will:
//   - Extract a reference to the locals block from the parsed file
//   - Continuously evaluate the block until all references are evaluated, defering evaluation of anything that references
//     other locals until those references are evaluated.
//
// This returns a map of the local names to the evaluated expressions (represented as `cty.Value` objects). This will
// error if there are remaining unevaluated locals after all references that can be evaluated has been evaluated.
func EvaluateLocalsBlock(ctx *ParsingContext, file *hclparse.File) (map[string]cty.Value, error) {
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
		ctx.TerragruntOptions.Logger.Debugf("Not all locals could be evaluated:")
		var errs *multierror.Error
		for _, attr := range attrs {
			diags := canEvaluateLocals(attr.Expr, evaluatedLocals)
			if err := file.HandleDiagnostics(diags); err != nil {
				errs = multierror.Append(errs, err)
			}
		}

		if err := errs.ErrorOrNil(); err != nil {
			return nil, errors.WithStackTrace(CouldNotEvaluateAllLocalsError{Err: err})
		}
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
		if diags := canEvaluateLocals(attr.Expr, evaluatedLocals); !diags.HasErrors() {
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
func canEvaluateLocals(expression hcl.Expression, evaluatedLocals map[string]cty.Value) hcl.Diagnostics {
	var diags hcl.Diagnostics

	localVars := expression.Variables()

	for _, localVar := range localVars {
		var (
			rootName        = localVar.RootName()
			localName       = getLocalName(localVar)
			_, hasEvaluated = evaluatedLocals[localName]
			detail          string
		)

		switch {
		case localVar.IsRelative():
			// This should never happen, but if it does, we can't evaluate this expression.
			detail = "This caused an impossible condition, tnis is almost certainly a bug in Terragrunt. Please open an issue at github.com/gruntwork-io/terragrunt with this message and the contents of your terragrunt.hcl file that caused this."

		case rootName == MetadataInclude:
			// If the variable is `include`, then we can evaluate it now

		case rootName != "local":
			// We can't evaluate any variable other than `local`
			detail = fmt.Sprintf("You can only reference to other local variables here, but it looks like you're referencing something else (%q is not defined)", rootName)

		case localName == "":
			// If we can't get any local name, we can't evaluate it.
			detail = "This local var name can not be determined."

		case !hasEvaluated:
			// If the referenced local isn't evaluated, we can't evaluate this expression.
			detail = fmt.Sprintf("The local reference '%s' is not evaluated. Either it is not ready yet in the current pass, or there was an error evaluating it in an earlier stage.", localName)
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

type CouldNotEvaluateAllLocalsError struct {
	Err error
}

func (err CouldNotEvaluateAllLocalsError) Error() string {
	return "Could not evaluate all locals in block."
}

func (err CouldNotEvaluateAllLocalsError) Unwrap() error {
	return err.Err
}

type MaxIterError struct{}

func (err MaxIterError) Error() string {
	return "Maximum iterations reached in attempting to evaluate locals. This is most likely a bug in Terragrunt. Please file an issue on the project: https://github.com/gruntwork-io/terragrunt/issues"
}
