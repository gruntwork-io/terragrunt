package config

// Engine represents the structure of the HCL data
type Engine struct {
	Source  string                 `hcl:"source"`
	Version string                 `hcl:"version"`
	Type    string                 `hcl:"type"`
	Meta    map[string]interface{} `hcl:"meta"`
}
