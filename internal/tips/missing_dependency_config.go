package tips

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GiveMissingDependencyConfigTip emits the MissingDependencyConfig tip for the component at path,
// which exists at ref but was deleted or moved in the Git diff.
func GiveMissingDependencyConfigTip(l log.Logger, path, ref string, allTips Tips) {
	allTips.Find(MissingDependencyConfig).EvaluateWith(l, fmt.Sprintf(
		"A `dependency` block points at %s, which exists at %s but was deleted or moved in the Git diff. %s",
		path,
		ref,
		missingDependencyConfigFix,
	))
}
