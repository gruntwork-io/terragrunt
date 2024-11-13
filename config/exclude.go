package config

// ExcludeConfig configurations for hcl files.
type ExcludeConfig struct {
	If                  bool     `cty:"if" hcl:"if,attr"`
	Actions             []string `cty:"actions" hcl:"actions,attr"`
	ExcludeDependencies bool     `cty:"exclude_dependencies" hcl:"exclude_dependencies,attr"`
}

// Clone returns a new instance of ExcludeConfig with the same values as the original.
func (e *ExcludeConfig) Clone() *ExcludeConfig {
	return &ExcludeConfig{
		If:                  e.If,
		Actions:             e.Actions,
		ExcludeDependencies: e.ExcludeDependencies,
	}
}
