package tips

import (
	"errors"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GiveMissingDependencyConfigTip emits the MissingDependencyConfig tip when err reports a missing Terragrunt config.
func GiveMissingDependencyConfigTip(l log.Logger, err error, allTips Tips) {
	_, notFound := errors.AsType[config.TerragruntConfigNotFoundError](err)
	_, depNotFound := errors.AsType[config.DependencyConfigNotFound](err)

	if notFound || depNotFound {
		allTips.Find(MissingDependencyConfig).Evaluate(l)
	}
}
