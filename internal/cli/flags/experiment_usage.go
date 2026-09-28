package flags

import (
	"github.com/gruntwork-io/terragrunt/internal/experiment"
)

// ExperimentUsage returns usage with a note that the flag needs the named
// experiment, while that experiment is still ongoing. Once it graduates the
// note disappears on its own, so a flag cannot go on advertising a gate that
// no longer holds.
//
// Build every gated flag's usage through here rather than writing the
// sentence into the string: the two are then impossible to update out of
// step, and moving [experiment.Experiment.Status] to
// [experiment.StatusCompleted] is the only edit graduation needs.
func ExperimentUsage(exps experiment.Experiments, name, usage string) string {
	exp := exps.Find(name)
	if exp == nil || exp.Status != experiment.StatusOngoing {
		return usage
	}

	return usage + " Requires the '" + name + "' experiment."
}
