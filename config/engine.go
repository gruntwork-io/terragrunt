package config

// EngineConfig represents the structure of the HCL data
type EngineConfig struct {
	Source  string                 `hcl:"source" cty:"source"`
	Version string                 `hcl:"version" cty:"version"`
	Type    string                 `hcl:"type" cty:"type"`
	Meta    map[string]interface{} `hcl:"meta" cty:"meta"`
}
