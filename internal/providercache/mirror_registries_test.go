package providercache_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/providercache"
	pcoptions "github.com/gruntwork-io/terragrunt/internal/providercache/options"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendMirroredRegistries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		methods       cliconfig.ProviderInstallationMethods
		registryNames []string
		expected      []string
	}{
		{
			name:          "no methods",
			registryNames: []string{"registry.opentofu.org"},
			expected:      []string{"registry.opentofu.org"},
		},
		{
			name: "network mirror include adds its registry once",
			methods: cliconfig.ProviderInstallationMethods{
				cliconfig.NewProviderInstallationNetworkMirror(
					"https://mirror.example.com/providers/",
					[]string{"registry.terraform.io/example/private", "registry.terraform.io/example/other"},
					nil,
				),
			},
			registryNames: []string{"registry.opentofu.org"},
			expected:      []string{"registry.opentofu.org", "registry.terraform.io"},
		},
		{
			name: "filesystem mirror include adds its registry",
			methods: cliconfig.ProviderInstallationMethods{
				cliconfig.NewProviderInstallationFilesystemMirror(
					"/mirror",
					[]string{"example.com/*/*"},
					nil,
				),
			},
			registryNames: []string{"registry.opentofu.org"},
			expected:      []string{"registry.opentofu.org", "example.com"},
		},
		{
			name: "already routed registry is kept once",
			methods: cliconfig.ProviderInstallationMethods{
				cliconfig.NewProviderInstallationNetworkMirror(
					"https://mirror.example.com/providers/",
					[]string{"registry.opentofu.org/example/private"},
					nil,
				),
			},
			registryNames: []string{"registry.opentofu.org"},
			expected:      []string{"registry.opentofu.org"},
		},
		{
			name: "patterns without a registry and direct includes are ignored",
			methods: cliconfig.ProviderInstallationMethods{
				cliconfig.NewProviderInstallationNetworkMirror(
					"https://mirror.example.com/providers/",
					[]string{"*/example/private", "example/private"},
					nil,
				),
				cliconfig.NewProviderInstallationDirect([]string{"registry.terraform.io/*/*"}, nil),
			},
			registryNames: []string{"registry.opentofu.org"},
			expected:      []string{"registry.opentofu.org"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := providercache.AppendMirroredRegistries(tt.methods, tt.registryNames)

			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestProviderCacheRoutesMirroredRegistries reproduces
// https://github.com/gruntwork-io/terragrunt/issues/7100: a provider that the user
// config fetches from a network_mirror is addressed through registry.terraform.io,
// which the cache server does not route for OpenTofu. The generated CLI config drops
// the user's mirror, so the request must reach the cache server, where the mirror is
// applied, and the second init must find the provider in the cache directory.
func TestProviderCacheRoutesMirroredRegistries(t *testing.T) {
	t.Parallel()

	const (
		home     = "/virtual/home"
		workDir  = "/virtual/work"
		cacheDir = "/virtual/provider-cache"
		token    = "11111111-2222-3333-4444-555555555555"
		registry = "registry.terraform.io"
	)

	var invocations []vexec.Invocation

	v := venvtest.New().
		WithGOOS("linux").
		WithUserHomeDir(func() (string, error) { return home, nil }).
		WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			invocations = append(invocations, inv)
			return vexec.Result{}
		})

	require.NoError(t, v.FS.MkdirAll(workDir, 0o755))
	require.NoError(t, v.FS.MkdirAll(home, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".tofurc"), []byte(`
credentials "private-registry.example.com" {
  token = "REDACTED"
}

provider_installation {
  network_mirror {
    url     = "https://private-registry.example.com/providers/"
    include = ["registry.terraform.io/example/private", "registry.opentofu.org/example/private"]
  }

  direct {
    exclude = ["registry.terraform.io/example/private", "registry.opentofu.org/example/private"]
  }
}
`), 0o600))

	pc := providercache.NewProviderCache()
	require.NoError(t, pc.Init(
		logger.CreateLogger(),
		v,
		tfimpl.OpenTofu,
		&pcoptions.ProviderCacheOptions{
			Dir:           cacheDir,
			Token:         token,
			RegistryNames: pcoptions.DefaultRegistryNames,
		},
		workDir,
	))

	generated := filepath.Join(workDir, ".terraformrc")

	for _, cacheRequestID := range []string{"request-id", ""} {
		require.NoError(t, pc.CreateLocalCLIConfig(t.Context(), v, tfimpl.OpenTofu, generated, cacheRequestID))

		cfg, err := cliconfig.LoadUserConfig(
			v.WithEnv(map[string]string{tf.EnvNameTFCLIConfigFile: generated}),
			tfimpl.OpenTofu,
		)
		require.NoError(t, err)

		hostNames := make([]string, 0, len(cfg.Hosts))
		for _, host := range cfg.Hosts {
			hostNames = append(hostNames, host.Name)
		}

		assert.Contains(t, hostNames, registry, "cache request id %q", cacheRequestID)

		if cacheRequestID != "" {
			continue
		}

		var mirrors []*cliconfig.ProviderInstallationFilesystemMirror

		for _, method := range cfg.ProviderInstallation.Methods {
			if mirror, ok := method.(*cliconfig.ProviderInstallationFilesystemMirror); ok {
				mirrors = append(mirrors, mirror)
			}
		}

		require.Len(t, mirrors, 1)
		require.NotNil(t, mirrors[0].Include)
		assert.Contains(t, *mirrors[0].Include, registry+"/*/*")
	}

	tfOpts := &tf.TFOptions{
		TerraformCliArgs:   iacargs.New(),
		ShellOptions:       shell.NewShellOptions(map[string]string{}).WithTFPath("tofu").WithWorkingDir(workDir),
		TofuImplementation: tfimpl.OpenTofu,
	}

	_, err := pc.TerraformCommandHook(t.Context(), logger.CreateLogger(), v, tfOpts, clihelper.Args{"init"})
	require.NoError(t, err)

	require.NotEmpty(t, invocations)

	for _, inv := range invocations {
		assert.Contains(
			t,
			strings.Join(inv.Env, "\x00"),
			"TF_TOKEN_registry_terraform_io="+providercache.APIKeyAuth+":"+token,
		)
	}
}
