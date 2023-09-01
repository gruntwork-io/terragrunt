package terraform

import (
	"os"
	"strings"

	"github.com/hashicorp/terraform/command/cliconfig"
)

// IsPluginCacheUsed returns true if the terraform plugin cache dir is specified, https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache
func IsPluginCacheUsed() bool {
	if strings.TrimSpace(os.Getenv("TF_PLUGIN_CACHE_DIR")) != "" {
		return true
	}

	cfg, _ := cliconfig.LoadConfig()
	return cfg.PluginCacheDir != ""
}
