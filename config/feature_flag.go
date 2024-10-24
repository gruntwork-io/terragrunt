package config

import "github.com/zclconf/go-cty/cty"

type FeatureFlags []FeatureFlag

type FeatureFlag struct {
	Name    string     `hcl:"name,attr"`
	Default *cty.Value `hcl:"default,attr" cty:"default"`
}

// DeepMerge merges the source FeatureFlag into the target FeatureFlag.
func (feature *FeatureFlag) DeepMerge(source FeatureFlag) error {
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
