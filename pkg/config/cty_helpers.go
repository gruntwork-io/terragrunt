//nolint:dupl
package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"dario.cat/mergo"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"maps"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
)

// Create a cty Function that takes as input parameters a slice of strings (var args, so this slice could be of any
// length) and returns as output a string. The implementation of the function calls the given toWrap function, passing
// it the input parameters string slice as well as the given include and terragruntOptions.
func wrapStringSliceToStringAsFuncImpl(
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	toWrap func(ctx context.Context, pctx *ParsingContext, l log.Logger, params []string) (string, error),
) function.Function {
	return function.New(&function.Spec{
		VarParam: &function.Parameter{Type: cty.String},
		Type:     function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			params, err := ctySliceToStringSlice(args)
			if err != nil {
				return cty.StringVal(""), err
			}

			out, err := toWrap(ctx, pctx, l, params)
			if err != nil {
				return cty.StringVal(""), err
			}

			return cty.StringVal(out), nil
		},
	})
}

func wrapStringSliceToNumberAsFuncImpl(
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	toWrap func(ctx context.Context, pctx *ParsingContext, l log.Logger, params []string) (int64, error),
) function.Function {
	return function.New(&function.Spec{
		VarParam: &function.Parameter{Type: cty.String},
		Type:     function.StaticReturnType(cty.Number),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			params, err := ctySliceToStringSlice(args)
			if err != nil {
				return cty.NumberIntVal(0), err
			}

			out, err := toWrap(ctx, pctx, l, params)
			if err != nil {
				return cty.NumberIntVal(0), err
			}

			return cty.NumberIntVal(out), nil
		},
	})
}

func wrapStringSliceToBoolAsFuncImpl(
	ctx context.Context,
	pctx *ParsingContext,
	toWrap func(ctx context.Context, pctx *ParsingContext, params []string) (bool, error),
) function.Function {
	return function.New(&function.Spec{
		VarParam: &function.Parameter{Type: cty.String},
		Type:     function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			params, err := ctySliceToStringSlice(args)
			if err != nil {
				return cty.BoolVal(false), err
			}

			out, err := toWrap(ctx, pctx, params)
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
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	toWrap func(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error),
) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			out, err := toWrap(ctx, pctx, l)
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
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	toWrap func(ctx context.Context, pctx *ParsingContext, l log.Logger) ([]string, error),
) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.List(cty.String)),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			outVals, err := toWrap(ctx, pctx, l)
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

// Create a cty Function that takes a string slice as input parameters and returns a string slice as output.
// The implementation of the function calls the given toWrap function.
func wrapStringSliceToStringSliceAsFuncImpl(
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	toWrap func(ctx context.Context, pctx *ParsingContext, l log.Logger, params []string) ([]string, error),
) function.Function {
	return function.New(&function.Spec{
		VarParam: &function.Parameter{Type: cty.String},
		Type:     function.StaticReturnType(cty.List(cty.String)),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			params, err := ctySliceToStringSlice(args)
			if err != nil {
				return cty.ListValEmpty(cty.String), err
			}

			outVals, err := toWrap(ctx, pctx, l, params)
			if err != nil || len(outVals) == 0 {
				return cty.ListValEmpty(cty.String), err
			}

			outCtyVals := make([]cty.Value, 0, len(outVals))
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
			outVals := make([]cty.Value, 0, len(out))
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
	var out = make([]string, 0, len(args))

	for _, arg := range args {
		if arg.Type() != cty.String {
			return nil, InvalidParameterTypeError{Expected: "string", Actual: arg.Type().FriendlyName()}
		}

		out = append(out, arg.AsString())
	}

	return out, nil
}

// shallowMergeCtyMaps performs a shallow merge of two cty value objects.
func shallowMergeCtyMaps(target cty.Value, source cty.Value) (*cty.Value, error) {
	outMap, err := ctyhelper.ParseCtyValueToMap(target)
	if err != nil {
		return nil, err
	}

	SourceMap, err := ctyhelper.ParseCtyValueToMap(source)
	if err != nil {
		return nil, err
	}

	for key, sourceValue := range SourceMap {
		if _, ok := outMap[key]; !ok {
			outMap[key] = sourceValue
		}
	}

	outCty, err := convertToCtyWithJSON(outMap)
	if err != nil {
		return nil, err
	}

	return &outCty, nil
}

func deepMergeCtyMaps(target cty.Value, source cty.Value) (*cty.Value, error) {
	return deepMergeCtyMapsMapOnly(target, source, mergo.WithAppendSlice)
}

// Create a cty Function that deeply merges map/object values.
// Later args override earlier args for overlapping keys.
func deepMergeMapValuesAsFuncImpl(pctx *ParsingContext) function.Function {
	return function.New(&function.Spec{
		VarParam: &function.Parameter{
			Type:             cty.DynamicPseudoType,
			AllowNull:        true,
			AllowDynamicType: true,
		},
		Type: function.StaticReturnType(cty.DynamicPseudoType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			if !pctx.Experiments.Evaluate(experiment.DeepMerge) {
				return cty.NilVal, DeepMergeRequiresExperimentError{ConfigPath: pctx.TerragruntConfigPath}
			}

			outVal := cty.EmptyObjectVal

			for _, arg := range args {
				if arg.IsNull() {
					continue
				}

				if !arg.Type().IsMapType() && !arg.Type().IsObjectType() {
					return cty.NilVal,
						InvalidParameterTypeError{Expected: "map or object", Actual: arg.Type().FriendlyName()}
				}

				merged, err := deepMergeCtyMaps(outVal, arg)
				if err != nil {
					return cty.NilVal, err
				}

				outVal = *merged
			}

			return outVal, nil
		},
	})
}

// deepMergeCtyMapsMapOnly implements a deep merge of two cty value objects. We can't directly merge two cty.Value objects, so
// we cheat by using map[string]any as an intermediary. Note that this assumes the provided cty value objects
// are already maps or objects in HCL land.
func deepMergeCtyMapsMapOnly(target cty.Value, source cty.Value, opts ...func(*mergo.Config)) (*cty.Value, error) {
	outMap := make(map[string]any)

	targetMap, err := ctyhelper.ParseCtyValueToMap(target)
	if err != nil {
		return nil, err
	}

	sourceMap, err := ctyhelper.ParseCtyValueToMap(source)
	if err != nil {
		return nil, err
	}

	maps.Copy(outMap, targetMap)

	if err := mergo.Merge(&outMap, sourceMap, append(opts, mergo.WithOverride)...); err != nil {
		return nil, err
	}

	outCty, err := convertToCtyWithJSON(outMap)
	if err != nil {
		return nil, err
	}

	return &outCty, nil
}

// ConvertValuesMapToCtyVal takes a map of name - cty.Value pairs and converts to a single cty.Value object.
func ConvertValuesMapToCtyVal(valMap map[string]cty.Value) (cty.Value, error) {
	if len(valMap) == 0 {
		// Return an empty object instead of NilVal for empty maps.
		return cty.EmptyObjectVal, nil
	}

	// Use cty.ObjectVal directly instead of gocty.ToCtyValue to preserve marks (like sensitive())
	return cty.ObjectVal(valMap), nil
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
func includeMapAsCtyVal(ctx context.Context, pctx *ParsingContext, l log.Logger) (cty.Value, error) {
	bareInclude, hasBareInclude := pctx.TrackInclude.CurrentMap[bareIncludeKey]
	if len(pctx.TrackInclude.CurrentMap) == 1 && hasBareInclude {
		l.Debug("Detected single bare include block - exposing as top level")
		return includeConfigAsCtyVal(ctx, pctx, l, bareInclude)
	}

	exposedIncludeMap := map[string]cty.Value{}

	for key, included := range pctx.TrackInclude.CurrentMap {
		parsedIncludedCty, err := includeConfigAsCtyVal(ctx, pctx, l, included)
		if err != nil {
			return cty.NilVal, err
		}

		if parsedIncludedCty != cty.NilVal {
			l.Debugf("Exposing include block '%s'", key)

			exposedIncludeMap[key] = parsedIncludedCty
		}
	}

	return ConvertValuesMapToCtyVal(exposedIncludeMap)
}

// includeBlockLabel returns a human-readable identifier for an include block to use in error messages:
// the quoted name for a named include, or "(bare include)" for the legacy unnamed include
// (whose Name is bareIncludeKey == "").
func includeBlockLabel(includeConfig IncludeConfig) string {
	if includeConfig.Name == bareIncludeKey {
		return "(bare include)"
	}

	return fmt.Sprintf("%q", includeConfig.Name)
}

// ctyPathString renders the cty.Path carried by a cty.PathError as a dotted/indexed attribute string
// (e.g. `.outputs["enabled"]` or `.list[1]`). It returns "" when err carries no cty.PathError or an empty path,
// so callers can omit the segment. The path is populated only when go-cty descended into a Go map/struct to
// reach the offending value; a top-level conversion such as gocty.ToCtyValue(v, cty.Bool) yields none.
func ctyPathString(err error) string {
	var pathErr cty.PathError
	if !errors.As(err, &pathErr) {
		return ""
	}

	var b strings.Builder

	for _, step := range pathErr.Path {
		switch s := step.(type) {
		case cty.GetAttrStep:
			b.WriteString("." + s.Name)
		case cty.IndexStep:
			// Let go-cty's JSON encoder render the key so we don't special-case string vs number keys.
			key, err := ctyjson.Marshal(s.Key, s.Key.Type())
			if err != nil {
				key = fmt.Appendf(nil, "%v", s.Key)
			}

			b.WriteString("[" + string(key) + "]")
		}
	}

	return b.String()
}

// fieldError annotates a config-field conversion error with the field name and, when go-cty can determine one,
// the failing attribute path within it, as a single dotted locator (e.g. `dependency.outputs["enabled"]`).
func fieldError(field string, err error) error {
	return fmt.Errorf("%s%s: %w", field, ctyPathString(err), err)
}

// includeConfigAsCtyVal returns the parsed include block as a cty.Value object if expose is true. Otherwise, return
// the nil representation of cty.Value.
func includeConfigAsCtyVal(ctx context.Context, pctx *ParsingContext, l log.Logger, includeConfig IncludeConfig) (cty.Value, error) {
	pctx = pctx.WithTrackInclude(nil)

	if includeConfig.GetExpose() {
		// Annotate resolution errors with the include name and parent file. The conversion layer further annotates
		// these with the failing field/attribute path (see TerragruntConfigAsCty), since low-level conversion errors
		// carry no source location of their own.
		parsedIncluded, err := parseIncludedConfig(ctx, pctx, l, &includeConfig)
		if err != nil {
			return cty.NilVal, fmt.Errorf("exposed include %s (%s): %w", includeBlockLabel(includeConfig), includeConfig.Path, err)
		}

		parsedIncludedCty, err := TerragruntConfigAsCty(parsedIncluded)
		if err != nil {
			return cty.NilVal, fmt.Errorf("exposed include %s (%s): %w", includeBlockLabel(includeConfig), includeConfig.Path, err)
		}

		return parsedIncludedCty, nil
	}

	return cty.NilVal, nil
}

// CtyToStruct converts a cty.Value to a go struct.
func CtyToStruct(ctyValue cty.Value, target any) error {
	jsonBytes, err := ctyjson.Marshal(ctyValue, ctyValue.Type())
	if err != nil {
		return err
	}

	if err := json.Unmarshal(jsonBytes, target); err != nil {
		return err
	}

	return nil
}

// CtyValueAsString converts a cty.Value to a string.
func CtyValueAsString(val cty.Value) (string, error) {
	jsonBytes, err := ctyjson.Marshal(val, val.Type())
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}

// GetValueString returns the string representation of a cty.Value.
// If the value is of type cty.String, it returns the raw string value directly.
// Otherwise, it falls back to converting the value to a JSON-formatted string
// using the CtyValueAsString helper function.
//
// Returns an error if the conversion fails.
func GetValueString(value cty.Value) (string, error) {
	if value.Type() == cty.String {
		return value.AsString(), nil
	}

	return CtyValueAsString(value)
}

// IsComplexType checks if a value is a complex data type that can't be used with raw output.
func IsComplexType(value cty.Value) bool {
	return value.Type().IsObjectType() || value.Type().IsMapType() ||
		value.Type().IsListType() || value.Type().IsTupleType() ||
		value.Type().IsSetType()
}

// GetFirstKey returns the first key from a map.
// This is a helper for maps that are known to have exactly one element.
func GetFirstKey(m map[string]cty.Value) string {
	for k := range m {
		return k
	}

	return ""
}
