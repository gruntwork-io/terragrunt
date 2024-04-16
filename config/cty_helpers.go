//nolint:dupl
package config

import (
	"encoding/json"

	"github.com/imdario/mergo"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/go-commons/errors"
)

// Create a cty Function that takes as input parameters a slice of strings (var args, so this slice could be of any
// length) and returns as output a string. The implementation of the function calls the given toWrap function, passing
// it the input parameters string slice as well as the given include and terragruntOptions.
func wrapStringSliceToStringAsFuncImpl(
	ctx *ParsingContext,
	toWrap func(ctx *ParsingContext, params []string) (string, error),
) function.Function {
	return function.New(&function.Spec{
		VarParam: &function.Parameter{Type: cty.String},
		Type:     function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			params, err := ctySliceToStringSlice(args)
			if err != nil {
				return cty.StringVal(""), err
			}
			out, err := toWrap(ctx, params)
			if err != nil {
				return cty.StringVal(""), err
			}
			return cty.StringVal(out), nil
		},
	})
}

func wrapStringSliceToNumberAsFuncImpl(
	ctx *ParsingContext,
	toWrap func(ctx *ParsingContext, params []string) (int64, error),
) function.Function {
	return function.New(&function.Spec{
		VarParam: &function.Parameter{Type: cty.String},
		Type:     function.StaticReturnType(cty.Number),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			params, err := ctySliceToStringSlice(args)
			if err != nil {
				return cty.NumberIntVal(0), err
			}
			out, err := toWrap(ctx, params)
			if err != nil {
				return cty.NumberIntVal(0), err
			}
			return cty.NumberIntVal(out), nil
		},
	})
}

func wrapStringSliceToBoolAsFuncImpl(
	ctx *ParsingContext,
	toWrap func(ctx *ParsingContext, params []string) (bool, error),
) function.Function {
	return function.New(&function.Spec{
		VarParam: &function.Parameter{Type: cty.String},
		Type:     function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			params, err := ctySliceToStringSlice(args)
			if err != nil {
				return cty.BoolVal(false), err
			}
			out, err := toWrap(ctx, params)
			if err != nil {
				return cty.BoolVal(false), err
			}
			return cty.BoolVal(out), nil
		},
	})
}

// Create a cty Function that takes no input parameters and returns as output a string. The implementation of the
// function calls the given toWrap function, passing it the given include and terragruntOptions.
func wrapVoidToStringAsFuncImpl(
	ctx *ParsingContext,
	toWrap func(ctx *ParsingContext) (string, error),
) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			out, err := toWrap(ctx)
			if err != nil {
				return cty.StringVal(""), err
			}
			return cty.StringVal(out), nil
		},
	})
}

// Create a cty Function that takes no input parameters and returns as output an empty string.
func wrapVoidToEmptyStringAsFuncImpl() function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			return cty.StringVal(""), nil
		},
	})
}

// Create a cty Function that takes no input parameters and returns as output a string slice. The implementation of the
// function calls the given toWrap function, passing it the given include and terragruntOptions.
func wrapVoidToStringSliceAsFuncImpl(
	ctx *ParsingContext,
	toWrap func(ctx *ParsingContext) ([]string, error),
) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.List(cty.String)),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			outVals, err := toWrap(ctx)
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
			return nil, errors.WithStackTrace(InvalidParameterTypeError{Expected: "string", Actual: arg.Type().FriendlyName()})
		}
		out = append(out, arg.AsString())
	}
	return out, nil
}

// shallowMergeCtyMaps performs a shallow merge of two cty value objects.
func shallowMergeCtyMaps(target cty.Value, source cty.Value) (*cty.Value, error) {
	outMap, err := parseCtyValueToMap(target)
	if err != nil {
		return nil, err
	}
	SourceMap, err := parseCtyValueToMap(source)
	if err != nil {
		return nil, err
	}

	for key, sourceValue := range SourceMap {
		if _, ok := outMap[key]; !ok {
			outMap[key] = sourceValue
		}
	}

	outCty, err := convertToCtyWithJson(outMap)
	if err != nil {
		return nil, err
	}
	return &outCty, nil
}

func deepMergeCtyMaps(target cty.Value, source cty.Value) (*cty.Value, error) {
	return deepMergeCtyMapsMapOnly(target, source, mergo.WithAppendSlice)
}

// deepMergeCtyMapsMapOnly implements a deep merge of two cty value objects. We can't directly merge two cty.Value objects, so
// we cheat by using map[string]interface{} as an intermediary. Note that this assumes the provided cty value objects
// are already maps or objects in HCL land.
func deepMergeCtyMapsMapOnly(target cty.Value, source cty.Value, opts ...func(*mergo.Config)) (*cty.Value, error) {
	outMap := make(map[string]interface{})
	targetMap, err := parseCtyValueToMap(target)
	if err != nil {
		return nil, err
	}
	sourceMap, err := parseCtyValueToMap(source)
	if err != nil {
		return nil, err
	}

	for key, val := range targetMap {
		outMap[key] = val
	}

	if err := mergo.Merge(&outMap, sourceMap, append(opts, mergo.WithOverride)...); err != nil {
		return nil, err
	}

	outCty, err := convertToCtyWithJson(outMap)
	if err != nil {
		return nil, err
	}
	return &outCty, nil
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
	if len(valMap) > 0 {
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

// includeMapAsCtyVal converts the include map into a cty.Value struct that can be exposed to the child config. For
// backward compatibility, this function will return the included config object if the config only defines a single bare
// include block that is exposed.
// NOTE: When evaluated in a partial parse ctx, only the partially parsed ctx is available in the expose. This
// ensures that we can parse the child config without having access to dependencies when constructing the dependency
// graph.
func includeMapAsCtyVal(ctx *ParsingContext) (cty.Value, error) {
	bareInclude, hasBareInclude := ctx.TrackInclude.CurrentMap[bareIncludeKey]
	if len(ctx.TrackInclude.CurrentMap) == 1 && hasBareInclude {
		ctx.TerragruntOptions.Logger.Debug("Detected single bare include block - exposing as top level")
		return includeConfigAsCtyVal(ctx, bareInclude)
	}

	exposedIncludeMap := map[string]cty.Value{}
	for key, included := range ctx.TrackInclude.CurrentMap {
		parsedIncludedCty, err := includeConfigAsCtyVal(ctx, included)
		if err != nil {
			return cty.NilVal, err
		}
		if parsedIncludedCty != cty.NilVal {
			ctx.TerragruntOptions.Logger.Debugf("Exposing include block '%s'", key)
			exposedIncludeMap[key] = parsedIncludedCty
		}
	}
	return convertValuesMapToCtyVal(exposedIncludeMap)
}

// includeConfigAsCtyVal returns the parsed include block as a cty.Value object if expose is true. Otherwise, return
// the nil representation of cty.Value.
func includeConfigAsCtyVal(ctx *ParsingContext, includeConfig IncludeConfig) (cty.Value, error) {
	ctx = ctx.WithTrackInclude(nil)

	if includeConfig.GetExpose() {
		parsedIncluded, err := parseIncludedConfig(ctx, &includeConfig)
		if err != nil {
			return cty.NilVal, err
		}
		parsedIncludedCty, err := TerragruntConfigAsCty(parsedIncluded)
		if err != nil {
			return cty.NilVal, err
		}
		return parsedIncludedCty, nil
	}
	return cty.NilVal, nil
}

// updateUnknownCtyValValues updates unknown values with default value
func updateUnknownCtyValValues(value *cty.Value) (*cty.Value, error) {
	updatedValue := map[string]cty.Value{}

	for key, value := range value.AsValueMap() {
		if value.IsKnown() {
			updatedValue[key] = value
		} else {
			updatedValue[key] = cty.StringVal("")
		}
	}

	res, err := gocty.ToCtyValue(updatedValue, value.Type())
	if err != nil {
		return nil, err
	}

	return &res, nil
}
