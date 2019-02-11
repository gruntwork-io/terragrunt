package models

// IncludeConfig represents the configuration settings for a parent Terragrunt configuration file that you can
// "include" in a child Terragrunt configuration file
type IncludeConfig struct {
	Path string `hcl:"path"`
}

func (includeConfig *IncludeConfig) Clone() *IncludeConfig {
	ret := &IncludeConfig{
		Path: includeConfig.Path,
	}
	return ret
}
