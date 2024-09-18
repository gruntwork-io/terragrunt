// Package strict provides utilities used by Terragrunt to support a "strict" mode.
// By default strict mode is disabled, but when enabled, any breaking changes
// to Terragrunt behavior that is not backwards compatible will result in an error.
package strict

import "errors"

type StrictFeature struct {
	EnabledByDefault bool
	Error            error
	Warning          string
}

const PlanAll featureName = "run-all"

type featureName string

var strictFeatures = map[featureName]StrictFeature{
	PlanAll: {
		Error:   errors.New("the `plan-all` is no longer supported. Use the `terragrunt run-all plan` command instead"),
		Warning: "The `plan-all` command is no longer supported. Use of `plan-all` will result in an error in a future version of Terragrunt < `1.0`. Use the `terragrunt run-all plan` command instead.",
	},
}

// GetStrictFeature returns the strict feature with the given name.
func GetStrictFeature(name featureName) (StrictFeature, bool) {
	feature, ok := strictFeatures[name]
	return feature, ok
}
