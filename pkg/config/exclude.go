package config

import (
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
)

// ExcludeConfig configurations for hcl files.
type ExcludeConfig struct {
	ExcludeDependencies *bool    `cty:"exclude_dependencies" hcl:"exclude_dependencies,attr" json:"exclude_dependencies"`
	NoRun               *bool    `cty:"no_run"               hcl:"no_run,attr"               json:"no_run"`
	Actions             []string `cty:"actions"              hcl:"actions,attr"              json:"actions"`
	If                  bool     `cty:"if"                   hcl:"if,attr"                   json:"if"`
}

// IsActionListed checks if the action is listed in the exclude block.
func (e *ExcludeConfig) IsActionListed(action string) bool {
	return runcfg.IsActionListedInExclude(e.Actions, action)
}

// Excludes reports whether the block excludes its unit from action. A block
// with a false `if` excludes nothing, whatever actions it lists.
func (e *ExcludeConfig) Excludes(action string) bool {
	return e.If && e.IsActionListed(action)
}

// ShouldPreventRun checks if the unit should be prevented from running based on the no_run attribute and current action.
func (e *ExcludeConfig) ShouldPreventRun(action string) bool {
	return runcfg.ShouldPreventRunBasedOnExclude(e.Actions, e.NoRun, e.If, action)
}

// Clone returns a new instance of ExcludeConfig with the same values as the original.
func (e *ExcludeConfig) Clone() *ExcludeConfig {
	return &ExcludeConfig{
		If:                  e.If,
		Actions:             e.Actions,
		ExcludeDependencies: e.ExcludeDependencies,
		NoRun:               e.NoRun,
	}
}

// Merge merges the values of the provided ExcludeConfig into the original.
func (e *ExcludeConfig) Merge(exclude *ExcludeConfig) {
	// copy not empty fields
	e.If = exclude.If
	if len(exclude.Actions) > 0 {
		e.Actions = exclude.Actions
	}

	e.ExcludeDependencies = exclude.ExcludeDependencies
	e.NoRun = exclude.NoRun
}
