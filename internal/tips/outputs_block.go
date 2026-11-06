package tips

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GiveOutputsBlockTip emits the OutputsBlock tip for the unit at configPath, which a dependency
// block reads although it declares no outputs block.
func GiveOutputsBlockTip(l log.Logger, configPath string, allTips Tips) {
	allTips.Find(OutputsBlock).EvaluateWith(l, fmt.Sprintf(
		"A `dependency` block reads %s, which declares no `outputs` block. %s",
		configPath,
		outputsBlockFix,
	))
}
