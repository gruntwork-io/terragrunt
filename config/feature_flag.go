package config

import (
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// FeatureFlags represents a list of feature flags.
type FeatureFlags []*FeatureFlag

// FeatureFlag feature flags struct.
type FeatureFlag struct {
	Name    string     `cty:"name"    hcl:",label"`
	Default *cty.Value `cty:"default" hcl:"default,attr"`
}

// ctyFeatureFlag struct used to pass FeatureFlag to cty.Value.
type ctyFeatureFlag struct {
	Name  string    `cty:"name"`
	Value cty.Value `cty:"value"`
}

// DeepMerge merges the source FeatureFlag into the target FeatureFlag.
func (feature *FeatureFlag) DeepMerge(source *FeatureFlag) error {
	if source.Name != "" {
		feature.Name = source.Name
	}

	if source.Default == nil {
		feature.Default = source.Default
	} else {
		updatedDefaults, err := deepMergeCtyMaps(*feature.Default, *source.Default)
		if err != nil {
			return err
		}

		feature.Default = updatedDefaults
	}

	return nil
}

// DeepMerge feature flags.
func deepMergeFeatureBlocks(targetFeatureFlags []*FeatureFlag, sourceFeatureFlags []*FeatureFlag) ([]*FeatureFlag, error) {
	if sourceFeatureFlags == nil && targetFeatureFlags == nil {
		return nil, nil
	}

	keys := make([]string, 0, len(targetFeatureFlags))

	featureBlocks := make(map[string]*FeatureFlag)

	for _, flag := range targetFeatureFlags {
		featureBlocks[flag.Name] = flag
		keys = append(keys, flag.Name)
	}

	for _, flag := range sourceFeatureFlags {
		sameKeyDep, hasSameKey := featureBlocks[flag.Name]
		if hasSameKey {
			sameKeyFlagPtr := sameKeyDep
			if err := sameKeyFlagPtr.DeepMerge(flag); err != nil {
				return nil, err
			}

			featureBlocks[flag.Name] = sameKeyFlagPtr
		} else {
			featureBlocks[flag.Name] = flag
			keys = append(keys, flag.Name)
		}
	}

	combinedFlags := make([]*FeatureFlag, 0, len(keys))
	for _, key := range keys {
		combinedFlags = append(combinedFlags, featureBlocks[key])
	}

	return combinedFlags, nil
}

// DefaultAsString returns the default value of the feature flag as a string.
func (feature *FeatureFlag) DefaultAsString() (string, error) {
	if feature.Default == nil {
		return "", nil
	}

	if feature.Default.Type() == cty.String {
		return feature.Default.AsString(), nil
	}

	// convert other types as json representation
	jsonBytes, err := ctyjson.Marshal(*feature.Default, feature.Default.Type())
	if err != nil {
		return "", errors.WithStack(err)
	}

	return string(jsonBytes), nil
}

// Convert generic flag value to cty.Value.
func flagToCtyValue(name string, value interface{}) (cty.Value, error) {
	ctyValue, err := goTypeToCty(value)
	if err != nil {
		return cty.NilVal, err
	}

	ctyFlag := ctyFeatureFlag{
		Name:  name,
		Value: ctyValue,
	}

	return goTypeToCty(ctyFlag)
}

// Convert a flag to a cty.Value using the provided cty.Type.
func flagToTypedCtyValue(name string, ctyType cty.Type, value interface{}) (cty.Value, error) {
	var flagValue = value
	if ctyType == cty.Bool {
		// convert value to boolean even if it is string
		parsedValue, err := strconv.ParseBool(fmt.Sprintf("%v", flagValue))
		if err != nil {
			return cty.NilVal, errors.WithStack(err)
		}

		flagValue = parsedValue
	}

	ctyOut, err := goTypeToCty(flagValue)
	if err != nil {
		return cty.NilVal, errors.WithStack(err)
	}

	ctyFlag := ctyFeatureFlag{
		Name:  name,
		Value: ctyOut,
	}

	return goTypeToCty(ctyFlag)
}
