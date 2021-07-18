package config

import (
	"encoding/json"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"io/ioutil"
	"path/filepath"
)

// Create a cty Function that takes as input parameters a slice of strings (var args, so this slice could be of any
// length) and returns as output a string. The implementation of the function calls the given toWrap function, passing
// it the input parameters string slice as well as the given include and terragruntOptions.
func wrapStringSliceToStringAsFuncImpl(toWrap func(params []string, include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error), include *IncludeConfig, terragruntOptions *options.TerragruntOptions) function.Function {
	return function.New(&function.Spec{
		VarParam: &function.Parameter{Type: cty.String},
		Type:     function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			params, err := ctySliceToStringSlice(args)
			if err != nil {
				return cty.StringVal(""), err
			}
			out, err := toWrap(params, include, terragruntOptions)
			if err != nil {
				return cty.StringVal(""), err
			}
			return cty.StringVal(out), nil
		},
	})
}

// Create a cty Function that takes as input parameters an object and returns as output a string.
// The implementation of the function calls the given toWrap function, passing
// it the input parameters string slice as well as the given include and terragruntOptions.
func wrapObjectToStringAsFuncImpl(handlerCall func() ProviderHandler) function.Function {
	handler := handlerCall()

	return function.New(&function.Spec{
		VarParam: &function.Parameter{Type: handler.Param},
		Type:     function.StaticReturnType(cty.String),
		Impl:     handler.Impl,
	})
}

// Create a cty Function that takes no input parameters and returns as output a string. The implementation of the
// function calls the given toWrap function, passing it the given include and terragruntOptions.
func wrapVoidToStringAsFuncImpl(toWrap func(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) (string, error), include *IncludeConfig, terragruntOptions *options.TerragruntOptions) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			out, err := toWrap(include, terragruntOptions)
			if err != nil {
				return cty.StringVal(""), err
			}
			return cty.StringVal(out), nil
		},
	})
}

func templateFileFunctionImpl(basePath string) function.Function {
	params := []function.Parameter{
		{
			Name: "path",
			Type: cty.String,
		},
		{
			Name: "vars",
			Type: cty.DynamicPseudoType,
		},
	}

	loadTmpl := func(filename string) (hcl.Expression, error) {
		absFileName := filepath.Join(basePath, filename)

		file, err := ioutil.ReadFile(absFileName)

		if err != nil {
			return nil, err
		}

		expr, diags := hclsyntax.ParseTemplate(file, filename, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return nil, diags
		}

		return expr, nil
	}

	renderTmpl := func(expr hcl.Expression, varsVal cty.Value) (cty.Value, error) {
		if varsTy := varsVal.Type(); !(varsTy.IsMapType() || varsTy.IsObjectType()) {
			return cty.DynamicVal, function.NewArgErrorf(1, "invalid vars value: must be a map") // or an object, but we don't strongly distinguish these most of the time
		}

		ctx := &hcl.EvalContext{
			Variables: varsVal.AsValueMap(),
		}

		// We require all of the variables to be valid HCL identifiers, because
		// otherwise there would be no way to refer to them in the template
		// anyway. Rejecting this here gives better feedback to the user
		// than a syntax error somewhere in the template itself.
		for n := range ctx.Variables {
			if !hclsyntax.ValidIdentifier(n) {
				// This error message intentionally doesn't describe _all_ of
				// the different permutations that are technically valid as an
				// HCL identifier, but rather focuses on what we might
				// consider to be an "idiomatic" variable name.
				return cty.DynamicVal, function.NewArgErrorf(1, "invalid template variable name %q: must start with a letter, followed by zero or more letters, digits, and underscores", n)
			}
		}

		// We'll pre-check references in the template here so we can give a
		// more specialized error message than HCL would by default, so it's
		// clearer that this problem is coming from a templatefile call.
		for _, traversal := range expr.Variables() {
			root := traversal.RootName()
			if _, ok := ctx.Variables[root]; !ok {
				return cty.DynamicVal, function.NewArgErrorf(1, "vars map does not contain key %q, referenced at %s", root, traversal[0].SourceRange())
			}
		}

		val, diags := expr.Value(ctx)
		if diags.HasErrors() {
			return cty.DynamicVal, diags
		}
		return val, nil
	}

	return function.New(&function.Spec{
		Params: params,
		Type: func(args []cty.Value) (cty.Type, error) {
			if !(args[0].IsKnown() && args[1].IsKnown()) {
				return cty.DynamicPseudoType, nil
			}

			// We'll render our template now to see what result type it produces.
			// A template consisting only of a single interpolation an potentially
			// return any type.
			expr, err := loadTmpl(args[0].AsString())
			if err != nil {
				return cty.DynamicPseudoType, err
			}

			// This is safe even if args[1] contains unknowns because the HCL
			// template renderer itself knows how to short-circuit those.
			val, err := renderTmpl(expr, args[1])
			return val.Type(), err
		},
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			expr, err := loadTmpl(args[0].AsString())
			if err != nil {
				return cty.DynamicVal, err
			}
			return renderTmpl(expr, args[1])
		},
	})
}

// Create a cty Function that takes no input parameters and returns as output a string slice. The implementation of the
// function calls the given toWrap function, passing it the given include and terragruntOptions.
func wrapVoidToStringSliceAsFuncImpl(toWrap func(include *IncludeConfig, terragruntOptions *options.TerragruntOptions) ([]string, error), include *IncludeConfig, terragruntOptions *options.TerragruntOptions) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.List(cty.String)),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			outVals, err := toWrap(include, terragruntOptions)
			if err != nil || len(outVals) == 0 {
				return cty.ListValEmpty(cty.String), err
			}
			outCtyVals := []cty.Value{}
			for _, val := range outVals {
				outCtyVals = append(outCtyVals, cty.StringVal(val))
			}
			return cty.ListVal(outCtyVals), nil
		},
	})
}

// Create a cty Function that takes no input parameters and returns as output a string slice. The implementation of the
// function returns the given string slice.
func wrapStaticValueToStringSliceAsFuncImpl(out []string) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.List(cty.String)),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			outVals := []cty.Value{}
			for _, val := range out {
				outVals = append(outVals, cty.StringVal(val))
			}
			return cty.ListVal(outVals), nil
		},
	})
}

// Convert the slice of cty values to a slice of strings. If any of the values in the given slice is not a string,
// return an error.
func ctySliceToStringSlice(args []cty.Value) ([]string, error) {
	var out []string
	for _, arg := range args {
		if arg.Type() != cty.String {
			return nil, errors.WithStackTrace(InvalidParameterType{Expected: "string", Actual: arg.Type().FriendlyName()})
		}
		out = append(out, arg.AsString())
	}
	return out, nil
}

// This is a hacky workaround to convert a cty Value to a Go map[string]interface{}. cty does not support this directly
// (https://github.com/hashicorp/hcl2/issues/108) and doing it with gocty.FromCtyValue is nearly impossible, as cty
// requires you to specify all the output types and will error out when it hits interface{}. So, as an ugly workaround,
// we convert the given value to JSON using cty's JSON library and then convert the JSON back to a
// map[string]interface{} using the Go json library.
func parseCtyValueToMap(value cty.Value) (map[string]interface{}, error) {
	jsonBytes, err := ctyjson.Marshal(value, cty.DynamicPseudoType)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	var ctyJsonOutput CtyJsonOutput
	if err := json.Unmarshal(jsonBytes, &ctyJsonOutput); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return ctyJsonOutput.Value, nil
}

// When you convert a cty value to JSON, if any of that types are not yet known (i.e., are labeled as
// DynamicPseudoType), cty's Marshall method will write the type information to a type field and the actual value to
// a value field. This struct is used to capture that information so when we parse the JSON back into a Go struct, we
// can pull out just the Value field we need.
type CtyJsonOutput struct {
	Value map[string]interface{}
	Type  interface{}
}

// convertValuesMapToCtyVal takes a map of name - cty.Value pairs and converts to a single cty.Value object.
func convertValuesMapToCtyVal(valMap map[string]cty.Value) (cty.Value, error) {
	valMapAsCty := cty.NilVal
	if valMap != nil && len(valMap) > 0 {
		var err error
		valMapAsCty, err = gocty.ToCtyValue(valMap, generateTypeFromValuesMap(valMap))
		if err != nil {
			return valMapAsCty, errors.WithStackTrace(err)
		}
	}
	return valMapAsCty, nil
}

// generateTypeFromValuesMap takes a values map and returns an object type that has the same number of fields, but
// bound to each type of the underlying evaluated expression. This is the only way the HCL decoder will be happy, as
// object type is the only map type that allows different types for each attribute (cty.Map requires all attributes to
// have the same type.
func generateTypeFromValuesMap(valMap map[string]cty.Value) cty.Type {
	outType := map[string]cty.Type{}
	for k, v := range valMap {
		outType[k] = v.Type()
	}
	return cty.Object(outType)
}
