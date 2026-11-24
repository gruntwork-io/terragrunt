package tips

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GiveValuesBlockTip emits the ValuesBlock tip for read, a read of a value inside try() in a config without a values
// block.
func GiveValuesBlockTip(l log.Logger, read config.TryDefault, allTips Tips) {
	allTips.Find(ValuesBlock).EvaluateWith(l, fmt.Sprintf(
		"The %s at %s reads `values.%s` inside `try()` to fall back to a default. %s",
		read.Kind,
		read.Source,
		read.Name,
		valuesBlockFix,
	))
}
