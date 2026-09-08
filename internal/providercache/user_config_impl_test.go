package providercache_test

import (
	"bytes"
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
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitReadsImplementationConfigFiles is the wiring-level regression test for
// https://github.com/gruntwork-io/terragrunt/issues/6787: a ~/.tofurc network_mirror
// must not leak into the CLI config generated for a Terraform run.
func TestInitReadsImplementationConfigFiles(t *testing.T) {
	t.Parallel()

	const (
		home   = "/virtual/home"
		mirror = "https://mirror.example.test/providers/"
	)

	tc := []struct {
		name       string
		impl       tfimpl.Type
		wantMirror bool
	}{
		{name: "terraform ignores .tofurc", impl: tfimpl.Terraform, wantMirror: false},
		{name: "tofu reads .tofurc", impl: tfimpl.OpenTofu, wantMirror: true},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New().
				WithGOOS("linux").
				WithUserHomeDir(func() (string, error) { return home, nil })

			require.NoError(t, v.FS.MkdirAll(home, 0o755))
			require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(home, ".tofurc"), []byte(`
provider_installation {
  network_mirror {
    url = "`+mirror+`"
  }
}
`), 0o600))

			pc := providercache.NewProviderCache()
			require.NoError(t, pc.Init(
				logger.CreateLogger(),
				v,
				tt.impl,
				&pcoptions.ProviderCacheOptions{
					Dir:           "/virtual/provider-cache",
					Token:         "11111111-2222-3333-4444-555555555555",
					RegistryNames: pcoptions.DefaultRegistryNames,
				},
				"/virtual/work",
			))

			generated := "/virtual/work/.terraformrc"
			require.NoError(t, pc.CreateLocalCLIConfig(t.Context(), v, tt.impl, generated, ""))

			written, err := vfs.ReadFile(v.FS, generated)
			require.NoError(t, err)

			if tt.wantMirror {
				assert.Contains(t, string(written), mirror)
				return
			}

			assert.NotContains(t, string(written), mirror)
		})
	}
}

// TestImplementationMismatchWarning pins when a mixed-implementation run is told the cache
// server's CLI config files do not match the binary the run uses.
func TestImplementationMismatchWarning(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name       string
		serverImpl tfimpl.Type
		runImpl    tfimpl.Type
		wantWarn   bool
	}{
		{name: "matching terraform", serverImpl: tfimpl.Terraform, runImpl: tfimpl.Terraform},
		{name: "matching tofu", serverImpl: tfimpl.OpenTofu, runImpl: tfimpl.OpenTofu},
		{name: "unknown server with tofu run reads the same files", serverImpl: tfimpl.Unknown, runImpl: tfimpl.OpenTofu},
		{name: "unknown run never warns", serverImpl: tfimpl.Terraform, runImpl: tfimpl.Unknown},
		{name: "empty run never warns", serverImpl: tfimpl.OpenTofu, runImpl: ""},
		{name: "terraform run against tofu server warns", serverImpl: tfimpl.OpenTofu, runImpl: tfimpl.Terraform, wantWarn: true},
		{name: "terraform run against unknown server warns", serverImpl: tfimpl.Unknown, runImpl: tfimpl.Terraform, wantWarn: true},
		{name: "tofu run against terraform server warns", serverImpl: tfimpl.Terraform, runImpl: tfimpl.OpenTofu, wantWarn: true},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			warning := providercache.ImplementationMismatchWarning(tt.serverImpl, tt.runImpl)

			if !tt.wantWarn {
				assert.Empty(t, warning)
				return
			}

			assert.Contains(t, warning, string(tt.runImpl))
			assert.Contains(t, warning, "--tf-path")
		})
	}
}

// TestTerraformCommandHookBypassesCacheOnImplementationMismatch pins that a run whose
// implementation reads different CLI config files than the server loaded skips the cache
// entirely: the command runs directly, without the generated CLI config forced on it.
func TestTerraformCommandHookBypassesCacheOnImplementationMismatch(t *testing.T) {
	t.Parallel()

	const workDir = "/virtual/work"

	var invocations []vexec.Invocation

	v := venvtest.New().
		WithGOOS("linux").
		WithUserHomeDir(func() (string, error) { return "/virtual/home", nil }).
		WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			invocations = append(invocations, inv)
			return vexec.Result{}
		})

	require.NoError(t, v.FS.MkdirAll(workDir, 0o755))

	pc := providercache.NewProviderCache()
	require.NoError(t, pc.Init(
		logger.CreateLogger(),
		v,
		tfimpl.OpenTofu,
		&pcoptions.ProviderCacheOptions{
			Dir:           "/virtual/provider-cache",
			Token:         "11111111-2222-3333-4444-555555555555",
			RegistryNames: pcoptions.DefaultRegistryNames,
		},
		workDir,
	))

	l, output := newBufferLogger()

	tfOpts := &tf.TFOptions{
		TerraformCliArgs:   iacargs.New(),
		ShellOptions:       shell.NewShellOptions(map[string]string{}).WithTFPath("terraform").WithWorkingDir(workDir),
		TofuImplementation: tfimpl.Terraform,
	}

	_, err := pc.TerraformCommandHook(t.Context(), l, v, tfOpts, clihelper.Args{"init"})
	require.NoError(t, err)

	assert.Contains(t, output.String(), "skips the provider cache")
	require.Len(t, invocations, 1, "the target command must run directly, exactly once")
	assert.NotContains(t, strings.Join(invocations[0].Env, "\x00"), "TF_CLI_CONFIG_FILE=/virtual/work")
	assert.False(t, vfs.Exists(v.FS, workDir+"/.terraformrc"), "no cache CLI config may be generated for a bypassed run")
}

// newBufferLogger returns a logger whose output the test can assert on.
func newBufferLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)

	return log.New(log.WithOutput(output), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter)), output
}
