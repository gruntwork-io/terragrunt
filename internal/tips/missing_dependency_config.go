package tips

import (
	"errors"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GiveMissingDependencyConfigTip emits the MissingDependencyConfig tip when err reports a dependency without a config.
func GiveMissingDependencyConfigTip(l log.Logger, err error, allTips Tips) {
	if !isMissingDependencyConfig(err) {
		return
	}

	allTips.Find(MissingDependencyConfig).Evaluate(l)
}

func isMissingDependencyConfig(err error) bool {
	if _, ok := errors.AsType[config.MissingDependencyConfigError](err); ok {
		return true
	}

	_, ok := errors.AsType[config.DependencyConfigNotFound](err)

	return ok
}
