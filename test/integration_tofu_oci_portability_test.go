//go:build tofu && (linux || darwin)

package test_test

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/ociregistry"
)

// tofuOCIPortabilityRepository is the repository the portability tests publish under.
const tofuOCIPortabilityRepository = "terraform-modules/vpc"

// tofuOCIPortabilityFiles is the module tree published once and consumed by both tools.
var tofuOCIPortabilityFiles = map[string]string{
	"main.tf": `output "root" {
  value = "root"
}
`,
	"modules/sub/main.tf": `output "sub" {
  value = "sub"
}
`,
}

// TestTofuOCIModulePortability pins that one oci source string yields byte-identical modules via tofu and Terragrunt.
func TestTofuOCIModulePortability(t *testing.T) {
	t.Parallel()

	// The manifest contract is the portability guarantee, so drift must fail here first.
	require.Equal(t, getter.ArtifactTypeModulePkg, ociregistry.ArtifactTypeModulePkg)
	require.Equal(t, getter.MediaTypeModuleZip, ociregistry.MediaTypeModuleZip)

	registry := ociregistry.Start(t)
	manifest := registry.PushModule(t, tofuOCIPortabilityRepository, "1.0.0", tofuOCIPortabilityFiles)
	registry.PushModule(t, tofuOCIPortabilityRepository, "latest", tofuOCIPortabilityFiles)

	base := "oci://" + registry.Address() + "/" + tofuOCIPortabilityRepository
	subTree := map[string]string{"main.tf": tofuOCIPortabilityFiles["modules/sub/main.tf"]}

	testCases := []struct {
		want map[string]string
		name string
		src  string
	}{
		{name: "tag pin", src: base + "?tag=1.0.0", want: tofuOCIPortabilityFiles},
		{name: "digest pin", src: base + "?digest=" + manifest.Digest.String(), want: tofuOCIPortabilityFiles},
		{name: "no query defaults to latest", src: base, want: tofuOCIPortabilityFiles},
		{name: "subdir selection", src: base + "//modules/sub?tag=1.0.0", want: subTree},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tofuTree := tofuOCIModuleTree(t, registry, tc.src)
			terragruntTree := terragruntOCIModuleTree(t, portabilityVenv(t, registry), tc.src)

			require.Equal(t, tc.want, tofuTree, "tofu must materialize exactly the published module view")
			require.Equal(t, tofuTree, terragruntTree, "one source string must yield byte-identical modules in both tools")
		})
	}
}

// TestTofuOCIMovedTagReResolution pins that clean installs after a tag moves serve the new content in both tools.
func TestTofuOCIMovedTagReResolution(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	registry.PushModule(t, tofuOCIPortabilityRepository, "1.0.0", tofuOCIPortabilityFiles)

	source := "oci://" + registry.Address() + "/" + tofuOCIPortabilityRepository + "?tag=1.0.0"

	// Each round installs clean; the CAS store under the process cache dir carries over, so a stale entry would surface.
	v := portabilityVenv(t, registry)

	require.Equal(t, tofuOCIPortabilityFiles, tofuOCIModuleTree(t, registry, source))
	require.Equal(t, tofuOCIPortabilityFiles, terragruntOCIModuleTree(t, v, source))

	// The moved tag drops the subdir, so stale cache content cannot hide as a subset match.
	movedFiles := map[string]string{
		"main.tf": `output "root" {
  value = "root-moved"
}
`,
	}
	registry.PushModule(t, tofuOCIPortabilityRepository, "1.0.0", movedFiles)

	tofuTree := tofuOCIModuleTree(t, registry, source)
	terragruntTree := terragruntOCIModuleTree(t, v, source)

	require.Equal(t, movedFiles, tofuTree, "tofu must serve the moved tag's content")
	require.Equal(t, tofuTree, terragruntTree, "a moved tag must stay portable, never a stale cache on either side")
}

// tofuOCIModuleTree installs source through the real tofu binary and returns the effective module tree.
func tofuOCIModuleTree(t *testing.T, registry *ociregistry.Registry, source string) map[string]string {
	t.Helper()

	workDir := t.TempDir()
	rootConfig := `module "vpc" {
  source = "` + source + `"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "main.tf"), []byte(rootConfig), 0o600))

	// An empty home and CLI config keep the pull anonymous; SSL_CERT_FILE trusts the test registry.
	home := t.TempDir()
	certFile := filepath.Join(home, "registry-cert.pem")
	require.NoError(t, os.WriteFile(certFile, registry.CertPEM(), 0o600))

	cliConfig := filepath.Join(home, "cli.tfrc")
	require.NoError(t, os.WriteFile(cliConfig, nil, 0o600))

	cmd := exec.CommandContext(t.Context(), helpers.TofuBinary, "get")
	cmd.Dir = workDir

	// os/exec deduplicates keeping later values, so these override any ambient copies.
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"TF_CLI_CONFIG_FILE="+cliConfig,
		"TF_DATA_DIR="+filepath.Join(workDir, ".terraform"),
		"SSL_CERT_FILE="+certFile,
	)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "tofu get must install the oci module: %s", output)

	return visibleModuleTree(t, tofuInstalledModuleDir(t, workDir))
}

// terragruntOCIModuleTree downloads source through Terragrunt's production path and returns the module tree.
func terragruntOCIModuleTree(t *testing.T, v *venv.Venv, source string) map[string]string {
	t.Helper()

	downloadDir := t.TempDir()
	sourceURL, err := url.Parse(source)
	require.NoError(t, err)

	src := &tf.Source{
		CanonicalSourceURL: sourceURL,
		DownloadDir:        downloadDir,
		WorkingDir:         downloadDir,
		// The version file lives outside the compared tree, since it is Terragrunt bookkeeping.
		VersionFile: filepath.Join(t.TempDir(), "version-file.txt"),
	}

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	opts.Experiments = experiment.NewExperiments()
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.OCI))

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	_, err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		l,
		v,
		src,
		configbridge.NewRunOptions(opts),
		cfg,
		report.NewReport(),
	)
	require.NoError(t, err)

	return visibleModuleTree(t, downloadDir)
}

// portabilityVenv builds a hermetic venv so a developer's own credentials cannot authenticate the pull.
func portabilityVenv(t *testing.T, registry *ociregistry.Registry) *venv.Venv {
	t.Helper()

	home := t.TempDir()
	v := venv.OSVenv().
		WithEnv(map[string]string{"HOME": home}).
		WithUserHomeDir(func() (string, error) { return home, nil })
	v.HTTP = registry.Client()

	return v
}

// tofuInstalledModuleDir reads the module manifest for the directory tofu actually resolved, which
// differs from the install root when the source selects a //subdir.
func tofuInstalledModuleDir(t *testing.T, workDir string) string {
	t.Helper()

	manifestBytes, err := os.ReadFile(filepath.Join(workDir, ".terraform", "modules", "modules.json"))
	require.NoError(t, err)

	var manifest struct {
		Modules []struct {
			Key string `json:"Key"`
			Dir string `json:"Dir"`
		} `json:"Modules"`
	}
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))

	for _, module := range manifest.Modules {
		if module.Key != "vpc" {
			continue
		}

		// A pinned absolute TF_DATA_DIR makes tofu record absolute module dirs.
		if filepath.IsAbs(module.Dir) {
			return module.Dir
		}

		return filepath.Join(workDir, module.Dir)
	}

	require.FailNow(t, "module vpc not present in tofu's module manifest")

	return ""
}

// visibleModuleTree returns the module's user-visible files keyed by slash-separated relative path.
func visibleModuleTree(t *testing.T, root string) map[string]string {
	t.Helper()

	tree := map[string]string{}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		// Dot-prefixed entries are tool bookkeeping, not module content.
		if strings.HasPrefix(entry.Name(), ".") && rel != "." {
			if entry.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		if entry.IsDir() {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		tree[filepath.ToSlash(rel)] = string(data)

		return nil
	})
	require.NoError(t, err)

	return tree
}
