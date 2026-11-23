//go:build !windows && tf

package test_test

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFAutoInitRunsWhenAValueADownloadedModuleSourceReadsChanges(t *testing.T) {
	t.Parallel()

	if helpers.IsTerraform(t.Context()) {
		t.Skip("Terraform evaluates only const variables in module sources")
		return
	}

	libraryDir := helpers.TmpDirWOSymlinks(t)
	writeFiles(t, libraryDir, map[string]string{
		"main.tf": `variable "flavor" {
  type = string
}

module "flavored" {
  source = "./flavors/${var.flavor}"
}
`,
		"flavors/small/main.tf": `output "size" { value = "small" }`,
		"flavors/large/main.tf": `output "size" { value = "large" }`,
	})
	helpers.CreateGitRepo(t, libraryDir)

	unitDir := helpers.TmpDirWOSymlinks(t)
	writeFiles(t, unitDir, map[string]string{
		"terragrunt.hcl": `inputs = { flavor = get_env("TG_TEST_AUTO_INIT_FLAVOR", "small") }`,
		"main.tf": fmt.Sprintf(`variable "flavor" {}

locals {
  flavor = var.flavor
}

module "library" {
  source = "git::file://%s"
  flavor = local.flavor
}
`, libraryDir),
	})

	recorder := helpers.NewExecRecorder(vexec.NewOSExec())

	plan := func(flavor string) bool {
		t.Helper()

		v := helpers.RunVenv(t).WithExec(recorder)
		env := maps.Clone(v.Env)
		env["TG_TEST_AUTO_INIT_FLAVOR"] = flavor

		return planRunsInit(t, v.WithEnv(env), recorder, "terragrunt plan --non-interactive --working-dir "+unitDir)
	}

	ranInit := []bool{plan("small"), plan("small"), plan("large"), plan("large")}

	assert.Equal(t, []bool{true, false, true, false}, ranInit)
}

// writeFiles writes each file of files, keyed by its slash-separated path under dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for path, content := range files {
		file := filepath.Join(dir, filepath.FromSlash(path))
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
		require.NoError(t, os.WriteFile(file, []byte(content), 0o644))
	}
}
