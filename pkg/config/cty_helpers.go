//nolint:dupl
package config

import (
	"context"
	"encoding/json"
	"maps"

	"dario.cat/mergo"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ConvertValuesMapToCtyVal takes a map of name - cty.Value pairs and converts to a single cty.Value object.
func ConvertValuesMapToCtyVal(valMap map[string]cty.Value) (cty.Value, error) {
	if len(valMap) == 0 {
		return cty.EmptyObjectVal, nil
	}

	return cty.ObjectVal(valMap), nil
}

// CtyToStruct converts a cty.Value to a go struct.
func CtyToStruct(ctyValue cty.Value, target any) error {
	jsonBytes, err := ctyjson.Marshal(ctyValue, ctyValue.Type())
	if err != nil {
		return errors.New(err)
	}

	if err := json.Unmarshal(jsonBytes, target); err != nil {
		return errors.New(err)
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
func GetFirstKey(m map[string]cty.Value) string {
	for k := range m {
		return k
	}

	return ""
}

// Private helper functions

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

			out, err := callWithPanicProtection(func() (string, error) {
				return toWrap(ctx, pctx, l, params)
			})
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

			out, err := callWithPanicProtection(func() (int64, error) {
				return toWrap(ctx, pctx, l, params)
			})
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

			out, err := callWithPanicProtection(func() (bool, error) {
				return toWrap(ctx, pctx, params)
			})
			if err != nil {
				return cty.BoolVal(false), err
			}

			return cty.BoolVal(out), nil
		},
	})
}

func wrapVoidToStringAsFuncImpl(
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	toWrap func(ctx context.Context, pctx *ParsingContext, l log.Logger) (string, error),
) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			out, err := callWithPanicProtection(func() (string, error) {
				return toWrap(ctx, pctx, l)
			})
			if err != nil {
				return cty.StringVal(""), err
			}

			return cty.StringVal(out), nil
		},
	})
}

func wrapVoidToEmptyStringAsFuncImpl() function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			return cty.StringVal(""), nil
		},
	})
}

func wrapVoidToStringSliceAsFuncImpl(
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	toWrap func(ctx context.Context, pctx *ParsingContext, l log.Logger) ([]string, error),
) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.List(cty.String)),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			outVals, err := callWithPanicProtection(func() ([]string, error) {
				return toWrap(ctx, pctx, l)
			})
			if err != nil {
				return cty.ListValEmpty(cty.String), err
			}

			if len(outVals) == 0 {
				return cty.ListValEmpty(cty.String), nil
			}

			outCtyVals := []cty.Value{}
			for _, val := range outVals {
				outCtyVals = append(outCtyVals, cty.StringVal(val))
			}

			return cty.ListVal(outCtyVals), nil
		},
	})
}

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

			outVals, err := callWithPanicProtection(func() ([]string, error) {
				return toWrap(ctx, pctx, l, params)
			})
			if err != nil {
				return cty.ListValEmpty(cty.String), err
			}

			if len(outVals) == 0 {
				return cty.ListValEmpty(cty.String), nil
			}

			outCtyVals := make([]cty.Value, 0, len(outVals))
			for _, val := range outVals {
				outCtyVals = append(outCtyVals, cty.StringVal(val))
			}

			return cty.ListVal(outCtyVals), nil
		},
	})
}

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

func ctySliceToStringSlice(args []cty.Value) ([]string, error) {
	var out = make([]string, 0, len(args))

	for _, arg := range args {
		if arg.Type() != cty.String {
			return nil, errors.New(InvalidParameterTypeError{Expected: "string", Actual: arg.Type().FriendlyName()})
		}

		out = append(out, arg.AsString())
	}

	return out, nil
}

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

func generateTypeFromValuesMap(valMap map[string]cty.Value) cty.Type {
	outType := map[string]cty.Type{}
	for k, v := range valMap {
		outType[k] = v.Type()
	}

	return cty.Object(outType)
}

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

func includeConfigAsCtyVal(ctx context.Context, pctx *ParsingContext, l log.Logger, includeConfig IncludeConfig) (cty.Value, error) {
	pctx = pctx.WithTrackInclude(nil)

	if !includeConfig.GetExpose() {
		return cty.NilVal, nil
	}

	parsedIncluded, err := parseIncludedConfig(ctx, pctx, l, &includeConfig)
	if err != nil {
		return cty.NilVal, err
	}

	parsedIncludedCty, err := TerragruntConfigAsCty(parsedIncluded)
	if err != nil {
		return cty.NilVal, err
	}

	return parsedIncludedCty, nil
}

func callWithPanicProtection[T any](f func() (T, error)) (out T, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.NewFunctionPanicError(recovered)
		}
	}()

	return f()
}
