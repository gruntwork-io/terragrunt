package backend

import (
	"reflect"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type Config map[string]any

// IsEqual returns true if the given comparableCfg is in any way different than what is configured for the backend.
func (cfg Config) IsEqual(comparableCfg Config, backendName string, logger log.Logger) bool {
	if len(cfg) == 0 && len(comparableCfg) == 0 {
		return true
	}

	existingConfigNonNil := copyExistingNotNullValues(comparableCfg, cfg)

	if reflect.DeepEqual(existingConfigNonNil, map[string]any(cfg)) {
		logger.Debugf("Backend %s has not changed.", backendName)

		return true
	}

	logger.Debugf("Backend config %s has changed from %s to %s", backendName, existingConfigNonNil, cfg)

	return false
}

// copyExistingNotNullValues copies the non-nil values from the existingMap to a new map
func copyExistingNotNullValues(existingMap, newMap map[string]any) map[string]any {
	if existingMap == nil {
		return nil
	}

	existingConfigNonNil := map[string]any{}

	for existingKey, existingValue := range existingMap {
		newValue, newValueIsSet := newMap[existingKey]
		if existingValue == nil && !newValueIsSet {
			continue
		}
		// if newValue and existingValue are both maps, we need to recursively copy the non-nil values
		if existingValueMap, existingValueIsMap := existingValue.(map[string]any); existingValueIsMap {
			if newValueMap, newValueIsMap := newValue.(map[string]any); newValueIsMap {
				existingValue = copyExistingNotNullValues(existingValueMap, newValueMap)
			}
		}

		existingConfigNonNil[existingKey] = existingValue
	}

	return existingConfigNonNil
}
