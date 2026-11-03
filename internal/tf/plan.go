package tf

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
)

// PlanChanges counts what a plan would do, the way the summary line of
// `tofu plan` counts it: a replace is one add and one destroy, and an import
// counts whether or not the resource also changes. Outputs counts the outputs
// whose values change.
type PlanChanges struct {
	Add     int `json:"add"`
	Change  int `json:"change"`
	Destroy int `json:"destroy"`
	Import  int `json:"import,omitempty"`
	Forget  int `json:"forget,omitempty"`
	Outputs int `json:"outputs,omitempty"`
}

// planJSON is the part of a plan's JSON representation, the document
// `tofu show -json` prints for a plan file, that the counts come from.
type planJSON struct {
	OutputChanges   map[string]planJSONChange `json:"output_changes"`
	ResourceChanges []struct {
		Change planJSONChange `json:"change"`
	} `json:"resource_changes"`
}

type planJSONChange struct {
	Importing *struct{} `json:"importing"`
	Actions   []string  `json:"actions"`
}

// DecodePlanChanges counts the changes in the plan JSON read from r.
func DecodePlanChanges(r io.Reader) (PlanChanges, error) {
	var plan planJSON

	if err := json.NewDecoder(r).Decode(&plan); err != nil {
		return PlanChanges{}, fmt.Errorf("decoding plan JSON: %w", err)
	}

	var changes PlanChanges

	for _, rc := range plan.ResourceChanges {
		if slices.Contains(rc.Change.Actions, "create") {
			changes.Add++
		}

		if slices.Contains(rc.Change.Actions, "update") {
			changes.Change++
		}

		if slices.Contains(rc.Change.Actions, "delete") {
			changes.Destroy++
		}

		if slices.Contains(rc.Change.Actions, "forget") {
			changes.Forget++
		}

		if rc.Change.Importing != nil {
			changes.Import++
		}
	}

	for _, oc := range plan.OutputChanges {
		if !slices.Equal(oc.Actions, []string{"no-op"}) {
			changes.Outputs++
		}
	}

	return changes, nil
}
