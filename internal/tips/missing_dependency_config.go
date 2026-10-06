package tips

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GiveMissingDependencyConfigTip emits the MissingDependencyConfig tip for the unit at unitPath,
// which exists at ref but not where a `dependency` block points.
func GiveMissingDependencyConfigTip(l log.Logger, allTips Tips, unitPath, ref string) {
	allTips.Find(MissingDependencyConfig).EvaluateWith(
		l,
		fmt.Sprintf("%s Deleted unit: %s (exists at %s).", MissingDependencyConfigMessage, unitPath, ref),
	)
}
