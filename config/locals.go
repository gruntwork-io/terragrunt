package config

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// MaxIter is the maximum number of depth we support in recursively evaluating locals.
const MaxIter = 1000

// Detailed error messages in diagnostics returned by parsing locals
const (
	// A consistent detail message for all "not a valid identifier" diagnostics. This is exactly the same as that returned
	// by terraform.
	badIdentifierDetail = "A name must start with a letter and may contain only letters, digits, underscores, and dashes."

	// A consistent error message for multiple locals block in terragrunt config (which is currently not supported)
	multipleLocalsBlockDetail = "Terragrunt currently does not support multiple locals blocks in a single config. Consolidate to a single locals block."
)

// Local represents a single local name binding. This holds the unevaluated expression, extracted from the parsed file
// (but before decoding) so that we can look for references to other locals before evaluating.
type Local struct {
	Name string
	Expr hcl.Expression
}

// evaluateLocalsBlock is a routine to evaluate the locals block in a way to allow references to other locals. This
// will:
//   - Extract a reference to the locals block from the parsed file
//   - Continuously evaluate the block until all references are evaluated, defering evaluation of anything that references
//     other locals until those references are evaluated.
//
// This returns a map of the local names to the evaluated expressions (represented as `cty.Value` objects). This will
// error if there are remaining unevaluated locals after all references that can be evaluated has been evaluated.
func evaluateLocalsBlock(
	terragruntOptions *options.TerragruntOptions,
	parser *hclparse.Parser,
	hclFile *hcl.File,
	filename string,
	trackInclude *TrackInclude,
	decodeList []PartialDecodeSectionType,
) (map[string]cty.Value, error) {
	diagsWriter := util.GetDiagnosticsWriter(terragruntOptions.Logger, parser)

	localsBlock, diags := getLocalsBlock(hclFile)
	if diags.HasErrors() {
		err := diagsWriter.WriteDiagnostics(diags)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		return nil, errors.WithStackTrace(diags)
	}
	if localsBlock == nil {
		// No locals block referenced in the file
		terragruntOptions.Logger.Debugf("Did not find any locals block: skipping evaluation.")
		return nil, nil
	}

	terragruntOptions.Logger.Debugf("Found locals block: evaluating the expressions.")

	locals, diags := decodeLocalsBlock(localsBlock)
	if diags.HasErrors() {
		terragruntOptions.Logger.Errorf("Encountered error while decoding locals block into name expression pairs.")
		err := diagsWriter.WriteDiagnostics(diags)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		return nil, errors.WithStackTrace(diags)
	}

	// Continuously attempt to evaluate the locals until there are no more locals to evaluate, or we can't evaluate
	// further.
	evaluatedLocals := map[string]cty.Value{}
	evaluated := true
	for iterations := 0; len(locals) > 0 && evaluated; iterations++ {
		if iterations > MaxIter {
			// Reached maximum supported iterations, which is most likely an infinite loop bug so cut the iteration
			// short an return an error.
			return nil, errors.WithStackTrace(MaxIterError{})
		}

		var err error
		locals, evaluatedLocals, evaluated, err = attemptEvaluateLocals(
			terragruntOptions,
			filename,
			locals,
			evaluatedLocals,
			trackInclude,
			decodeList,
			diagsWriter,
		)
		if err != nil {
			terragruntOptions.Logger.Errorf("Encountered error while evaluating locals in file %s", filename)
			return nil, err
		}
	}
	if len(locals) > 0 {
		// This is an error because we couldn't evaluate all locals
		terragruntOptions.Logger.Errorf("Not all locals could be evaluated:")
		for _, local := range locals {
			_, reason := canEvaluate(local.Expr, evaluatedLocals)
			terragruntOptions.Logger.Errorf("\t- %s [REASON: %s]", local.Name, reason)
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
	terragruntOptions *options.TerragruntOptions,
	filename string,
	locals []*Local,
	evaluatedLocals map[string]cty.Value,
	trackInclude *TrackInclude,
	decodeList []PartialDecodeSectionType,
	diagsWriter hcl.DiagnosticWriter,
) (unevaluatedLocals []*Local, newEvaluatedLocals map[string]cty.Value, evaluated bool, err error) {
	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(
				PanicWhileParsingConfig{
					RecoveredValue: recovered,
					ConfigFile:     filename,
				},
			)
		}
	}()

	evaluatedLocalsAsCty, err := convertValuesMapToCtyVal(evaluatedLocals)
	if err != nil {
		terragruntOptions.Logger.Errorf("Could not convert evaluated locals to the execution context to evaluate additional locals in file %s", filename)
		return nil, evaluatedLocals, false, err
	}
	evalCtx, err := CreateTerragruntEvalContext(
		filename,
		terragruntOptions,
		EvalContextExtensions{
			TrackInclude:           trackInclude,
			Locals:                 &evaluatedLocalsAsCty,
			PartialParseDecodeList: decodeList,
		},
	)
	if err != nil {
		terragruntOptions.Logger.Errorf("Could not convert include to the execution context to evaluate additional locals in file %s", filename)
		return nil, evaluatedLocals, false, err
	}

	// Track the locals that were evaluated for logging purposes
	newlyEvaluatedLocalNames := []string{}

	unevaluatedLocals = []*Local{}
	evaluated = false
	newEvaluatedLocals = map[string]cty.Value{}
	for key, val := range evaluatedLocals {
		newEvaluatedLocals[key] = val
	}
	for _, local := range locals {
		localEvaluated, _ := canEvaluate(local.Expr, evaluatedLocals)
		if localEvaluated {
			evaluatedVal, diags := local.Expr.Value(evalCtx)
			if diags.HasErrors() {
				err := diagsWriter.WriteDiagnostics(diags)
				if err != nil {
					return nil, nil, false, errors.WithStackTrace(err)
				}
				return nil, evaluatedLocals, false, errors.WithStackTrace(diags)
			}
			newEvaluatedLocals[local.Name] = evaluatedVal
			newlyEvaluatedLocalNames = append(newlyEvaluatedLocalNames, local.Name)
			evaluated = true
		} else {
			unevaluatedLocals = append(unevaluatedLocals, local)
		}
	}

	terragruntOptions.Logger.Debugf(
		"Evaluated %d locals (remaining %d): %s",
		len(newlyEvaluatedLocalNames),
		len(unevaluatedLocals),
		strings.Join(newlyEvaluatedLocalNames, ", "),
	)
	return unevaluatedLocals, newEvaluatedLocals, evaluated, nil
}

// canEvaluate determines if the local expression can be evaluated. An expression can be evaluated if one of the
// following is true:
// - It has no references to other locals.
// - It has references to other locals that have already been evaluated.
// Note that the second return value is a human friendly reason for why the expression can not be evaluated, and is
// useful for error reporting.
func canEvaluate(expression hcl.Expression,
	evaluatedLocals map[string]cty.Value,
) (bool, string) {
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
		if rootName == "include" {
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

// getLocalsBlock takes a parsed HCL file and extracts a reference to the `locals` block, if there is one defined.
func getLocalsBlock(hclFile *hcl.File) (*hcl.Block, hcl.Diagnostics) {
	localsSchema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			hcl.BlockHeaderSchema{Type: "locals"},
		},
	}
	// We use PartialContent here, because we are only interested in parsing out the locals block.
	parsedLocals, _, diags := hclFile.Body.PartialContent(localsSchema)
	extractedLocalsBlocks := []*hcl.Block{}
	for _, block := range parsedLocals.Blocks {
		if block.Type == "locals" {
			extractedLocalsBlocks = append(extractedLocalsBlocks, block)
		}
	}
	// We currently only support parsing a single locals block
	switch {
	case len(extractedLocalsBlocks) == 1:
		return extractedLocalsBlocks[0], diags
	case len(extractedLocalsBlocks) > 1:
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Multiple locals block",
			Detail:   multipleLocalsBlockDetail,
		})
		return nil, diags
	default:
		// No locals block parsed
		return nil, diags
	}
}

// decodeLocalsBlock loads the block into name expression pairs to assist with evaluation of the locals prior to
// evaluating the whole config. Note that this is exactly the same as
// terraform/configs/named_values.go:decodeLocalsBlock
func decodeLocalsBlock(localsBlock *hcl.Block) ([]*Local, hcl.Diagnostics) {
	attrs, diags := localsBlock.Body.JustAttributes()
	if len(attrs) == 0 {
		return nil, diags
	}

	locals := make([]*Local, 0, len(attrs))
	for name, attr := range attrs {
		if !hclsyntax.ValidIdentifier(name) {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid local value name",
				Detail:   badIdentifierDetail,
				Subject:  &attr.NameRange,
			})
		}

		locals = append(locals, &Local{
			Name: name,
			Expr: attr.Expr,
		})
	}
	return locals, diags
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
