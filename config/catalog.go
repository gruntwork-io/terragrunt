package config

import (
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/zclconf/go-cty/cty"
)

type CatalogConfig struct {
	URLs []string `hcl:"urls,attr" cty:"urls"`
}

func (conf *CatalogConfig) String() string {
	return fmt.Sprintf("Catalog{URLs = %v}", conf.URLs)
}

func (config *CatalogConfig) normalize(cofnigPath string) {
	configDir := filepath.Dir(cofnigPath)

	// transform relative paths to absolute ones
	for i, url := range config.URLs {
		url := filepath.Join(configDir, url)

		if files.FileExists(url) {
			config.URLs[i] = url
		}
	}
}

// ctyCatalogConfig is an alternate representation of CatalogConfig that converts internal blocks into a map that
// maps the name to the underlying struct, as opposed to a list representation.
type ctyCatalogConfig struct {
	URLs []string `cty:"urls"`
}

// Serialize CatalogConfig to a cty Value, but with maps instead of lists for the blocks.
func catalogConfigAsCty(config *CatalogConfig) (cty.Value, error) {
	if config == nil {
		return cty.NilVal, nil
	}

	configCty := ctyCatalogConfig{
		URLs: config.URLs,
	}

	return goTypeToCty(configCty)
}
