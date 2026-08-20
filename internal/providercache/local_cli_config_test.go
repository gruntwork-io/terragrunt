package providercache_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/providercache"
	pcoptions "github.com/gruntwork-io/terragrunt/internal/providercache/options"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateLocalCLIConfigStripsProxiedCredentials pins the wiring, not the stripping: drop
// the call to [providercache.StripProxiedCredentials] and no other test notices the real
// token returning to disk.
func TestCreateLocalCLIConfigStripsProxiedCredentials(t *testing.T) {
	const (
		registry      = "registry.example.test"
		realToken     = "REAL-REGISTRY-TOKEN"
		cacheToken    = "11111111-2222-3333-4444-555555555555"
		userCLIConfig = `
credentials "` + registry + `" {
  token = "` + realToken + `"
}

host "` + registry + `" {
  services = {
    "providers.v1" = "https://` + registry + `/v1/providers/"
  }
}
`
	)

	rootDir := t.TempDir()

	v := venv.OSVenv().WithHTTP(vhttp.NewNoNetworkClient())

	userCfgPath := filepath.Join(rootDir, "user.tfrc")
	require.NoError(t, vfs.WriteFile(v.FS, userCfgPath, []byte(userCLIConfig), 0o600))

	// Both the user config Init reads and the registry list are scoped to the fake host, so
	// resolving it never leaves the process.
	t.Setenv("TF_CLI_CONFIG_FILE", userCfgPath)

	pc := providercache.NewProviderCache()
	require.NoError(t, pc.Init(
		logger.CreateLogger(),
		v,
		&pcoptions.ProviderCacheOptions{
			Dir:           filepath.Join(rootDir, "providers"),
			Token:         cacheToken,
			RegistryNames: []string{registry},
		},
		rootDir,
	))

	generated := filepath.Join(t.TempDir(), ".terraformrc")
	require.NoError(t, pc.CreateLocalCLIConfig(t.Context(), v, tfimpl.OpenTofu, generated, ""))

	written, err := vfs.ReadFile(v.FS, generated)
	require.NoError(t, err)

	assert.NotContains(t, string(written), realToken)
	assert.NotContains(t, string(written), `credentials "`+registry+`"`)
}
