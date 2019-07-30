package config

import (
	"sort"
	"strings"

	"github.com/hashicorp/hcl2/hcl"
	"github.com/hashicorp/hcl2/hcl/hclsyntax"
	"github.com/hashicorp/hcl2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// A consistent detail message for all "not a valid identifier" diagnostics. This is exactly the same as that returned
// by terraform.
const badIdentifierDetail = "A name must start with a letter and may contain only letters, digits, underscores, and dashes."

// Local represents a single local name binding. This holds the unevaluated expression, extracted from the parsed file
// (but before decoding) so that we can look for references to other locals before evaluating.
type Local struct {
	Name string
	Expr hcl.Expression
}

// evaluateLocalsBlock is a routine to evaluate the locals block in a way to allow references to other locals. This
// will:
// - Extract a reference to the locals block from the parsed file
// - Continuously evaluate the block until all references are evaluated, defering evaluation of anything that references
//   other locals until those references are evaluated.
// This returns a map of the local names to the evaluated expressions (represented as `cty.Value` objects). This will
// error if there are remaining unevaluated locals after all references that can be evaluated has been evaluated.
func evaluateLocalsBlock(
	terragruntOptions *options.TerragruntOptions,
	parser *hclparse.Parser,
	hclFile *hcl.File,
	filename string,
) (map[string]cty.Value, error) {
	diagsWriter := util.GetDiagnosticsWriter(parser)

	localsBlock, diags := getLocalsBlock(hclFile)
	if diags.HasErrors() {
		diagsWriter.WriteDiagnostics(diags)
		return nil, errors.WithStackTrace(diags)
	}
	if localsBlock == nil {
		// No locals block referenced in the file
		terragruntOptions.Logger.Printf("Did not find any locals block: skipping evaluation.")
		return nil, nil
	}

	terragruntOptions.Logger.Printf("Found locals block: evaluating the expressions.")

	locals, diags := decodeLocalsBlock(localsBlock)
	if diags.HasErrors() {
		terragruntOptions.Logger.Printf("Encountered error while decoding locals block into name expression pairs.")
		diagsWriter.WriteDiagnostics(diags)
		return nil, errors.WithStackTrace(diags)
	}

	// Continuously attempt to evaluate the locals until there are no more locals to evaluate, or we can't evaluate
	// further.
	evaluatedLocals := map[string]cty.Value{}
	evaluated := true
	for len(locals) > 0 && evaluated {
		var err error
		locals, evaluatedLocals, evaluated, err = attemptEvaluateLocals(terragruntOptions, filename, locals, evaluatedLocals)
		if err != nil {
			terragruntOptions.Logger.Printf("Encountered error while evaluating locals.")
			return nil, err
		}
	}
	if len(locals) > 0 {
		// This is an error because we couldn't evaluate all locals
		terragruntOptions.Logger.Printf("Not all locals could be evaluated:")
		for _, local := range locals {
			terragruntOptions.Logger.Printf("\t- %s", local.Name)
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

	evaluatedLocalsAsCty, err := convertLocalsMapToCtyVal(evaluatedLocals)
	if err != nil {
		terragruntOptions.Logger.Printf("Could not convert evaluated locals to the execution context to evaluate additional locals")
		return nil, evaluatedLocals, false, err
	}
	evalCtx := CreateTerragruntEvalContext(filename, terragruntOptions, nil, evaluatedLocalsAsCty)

	// Track the locals that were evaluated for logging purposes
	newlyEvaluatedLocalNames := []string{}

	unevaluatedLocals = []*Local{}
	evaluated = false
	newEvaluatedLocals = map[string]cty.Value{}
	for key, val := range evaluatedLocals {
		newEvaluatedLocals[key] = val
	}
	for _, local := range locals {
		if canEvaluate(terragruntOptions, local.Expr, evaluatedLocals) {
			evaluatedVal, diags := local.Expr.Value(evalCtx)
			if diags.HasErrors() {
				// TODO Write out diagnostic errors
				return nil, evaluatedLocals, false, errors.WithStackTrace(diags)
			}
			newEvaluatedLocals[local.Name] = evaluatedVal
			newlyEvaluatedLocalNames = append(newlyEvaluatedLocalNames, local.Name)
			evaluated = true
		} else {
			unevaluatedLocals = append(unevaluatedLocals, local)
		}
	}

	terragruntOptions.Logger.Printf(
		"Evaluated %d locals (remaining %d): %s", len(newlyEvaluatedLocalNames), len(unevaluatedLocals), strings.Join(newlyEvaluatedLocalNames, ", "))
	return unevaluatedLocals, newEvaluatedLocals, evaluated, nil
}

// canEvaluate determines if the local expression can be evaluated. An expression can be evaluated if one of the
// following is true:
// - It has no references to other locals.
// - It has references to other locals that have already been evaluated.
func canEvaluate(
	terragruntOptions *options.TerragruntOptions,
	expression hcl.Expression,
	evaluatedLocals map[string]cty.Value,
) bool {
	vars := expression.Variables()
	if len(vars) == 0 {
		// If there are no local variable references, we can evaluate this expression.
		return true
	}

	for _, var_ := range vars {
		// This should never happen, but if it does, we can't evaluate this expression.
		if var_.IsRelative() {
			return false
		}

		// We can't evaluate any variable other than `local` here.
		if var_.RootName() != "local" {
			return false
		}

		// If we can't get any local name, we can't evaluate it.
		localName := getLocalName(terragruntOptions, var_)
		if localName == "" {
			return false
		}

		// If the referenced local isn't evaluated, we can't evaluate this expression.
		_, hasEvaluated := evaluatedLocals[localName]
		if !hasEvaluated {
			return false
		}
	}

	// If we made it this far, this means all the variables referenced are accounted for and we can evaluate this
	// expression.
	return true
}

// getLocalName takes a variable reference encoded as a HCL tree traversal that is rooted at the name `local` and
// returns the underlying variable lookup on the local map. If it is not a local name lookup, this will return empty
// string.
func getLocalName(terragruntOptions *options.TerragruntOptions, traversal hcl.Traversal) string {
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
	for _, block := range parsedLocals.Blocks {
		if block.Type == "locals" {
			return block, diags
		}
	}
	// No locals block parsed
	return nil, diags
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

// convertLocalsMapToCtyVal takes the name - cty.Value pairs evaluated out of the locals block and converts to a single
// cty.Value object that can be passed in as a Variable to the HCL evaluation context.
func convertLocalsMapToCtyVal(locals map[string]cty.Value) (*cty.Value, error) {
	var localsAsCty *cty.Value
	localsAsCty = nil
	if locals != nil && len(locals) > 0 {
		localsAsCtyRaw, err := gocty.ToCtyValue(locals, generateTypeFromLocalsMap(locals))
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		localsAsCty = &localsAsCtyRaw
	}
	return localsAsCty, nil
}

// generateTypeFromLocalsMap takes the locals map and returns an object type that has the same number of fields, but
// bound to each type of the underlying evaluated expression. This is the only way the HCL decoder will be happy, as
// object type is the only map type that allows different types for each attribute (cty.Map requires all attributes to
// have the same type.
func generateTypeFromLocalsMap(locals map[string]cty.Value) cty.Type {
	outType := map[string]cty.Type{}
	for k, v := range locals {
		outType[k] = v.Type()
	}
	return cty.Object(outType)
}

// Return the keys for the given locals map, sorted alphabetically
func keysOfLocalsMap(m map[string]cty.Value) []string {
	out := []string{}

	for key, _ := range m {
		out = append(out, key)
	}

	sort.Strings(out)

	return out
}

// ------------------------------------------------
// Custom Errors Returned by Functions in this Code
// ------------------------------------------------

type CouldNotEvaluateAllLocalsError struct{}

func (err CouldNotEvaluateAllLocalsError) Error() string {
	return "Could not evaluate all locals in block."
}
