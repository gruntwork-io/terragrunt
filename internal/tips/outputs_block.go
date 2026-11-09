package tips

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GiveOutputsBlockTip emits the OutputsBlock tip for the dependency target at configPath without an outputs block.
func GiveOutputsBlockTip(l log.Logger, configPath string, allTips Tips) {
	allTips.Find(OutputsBlock).EvaluateWith(l, fmt.Sprintf(
		"A `dependency` block reads %s, which declares no `outputs` block. %s",
		configPath,
		outputsBlockFix,
	))
}
