package config

type FeatureFlags []FeatureFlag

type FeatureFlag struct {
	Name    string `hcl:"name,attr"`
	Default *bool  `hcl:"enabled,attr" cty:"default"`
}

// Clone

// Merge
