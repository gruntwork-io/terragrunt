package backend

import (
	"reflect"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	configPathKey = "path"
)

type Config map[string]any

// Path returns the `patha field value.
func (cfg Config) Path() string {
	return getConfigValueByKey[string](cfg, configPathKey)
}

// IsEqual returns true if the given `targetCfg` config is in any way different than what is configured for the backend.
func (cfg Config) IsEqual(targetCfg Config, backendName string, logger log.Logger) bool {
	if len(cfg) == 0 && len(targetCfg) == 0 {
		return true
	}

	targetCfgNonNil := cfg.CopyNotNullValues(targetCfg)

	if reflect.DeepEqual(targetCfgNonNil, map[string]any(cfg)) {
		logger.Debugf("Backend %s has not changed.", backendName)

		return true
	}

	logger.Debugf("Backend config %s has changed from %s to %s", backendName, targetCfgNonNil, cfg)

	return false
}

// CopyNotNullValues copies the non-nil values from the `targetCfg` whose keys also exist in the `cfg` to the new map.
func (cfg Config) CopyNotNullValues(targetCfg map[string]any) map[string]any {
	if targetCfg == nil {
		return nil
	}

	targetCfgNonNil := map[string]any{}

	for existingKey, existingValue := range targetCfg {
		newValue, newValueIsSet := cfg[existingKey]
		if existingValue == nil && !newValueIsSet {
			continue
		}
		// if newValue and existingValue are both maps, we need to recursively copy the non-nil values
		if existingValueMap, existingValueIsMap := existingValue.(map[string]any); existingValueIsMap {
			if newValueMap, newValueIsMap := newValue.(map[string]any); newValueIsMap {
				existingValue = Config(newValueMap).CopyNotNullValues(existingValueMap)
			}
		}

		targetCfgNonNil[existingKey] = existingValue
	}

	return targetCfgNonNil
}

func getConfigValueByKey[T any](m map[string]any, key string) T {
	if val, ok := m[key]; ok {
		if val, ok := val.(T); ok {
			return val
		}
	}

	return *new(T)
}
