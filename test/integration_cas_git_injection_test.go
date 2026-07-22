//go:build !windows

package test_test

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCASGitSourceRefOptionInjection checks a git source ?ref= never runs a command during download.
func TestCASGitSourceRefOptionInjection(t *testing.T) {
	t.Parallel()

	marker := filepath.Join(helpers.TmpDirWOSymlinks(t), "injected")

	// ${IFS} avoids a literal space, which git rejects in a ref name.
	injectedRef := "--upload-pack=touch${IFS}" + marker

	repoDir := helpers.TmpDirWOSymlinks(t)
	mainTF := "terraform {\n  required_version = \">= 1.0.0\"\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.tf"), []byte(mainTF), 0o644))
	helpers.InitGitRepoWithBranchRef(t, repoDir, injectedRef)

	liveDir := helpers.TmpDirWOSymlinks(t)
	source := "git::file://" + repoDir + "?ref=" + url.QueryEscape(injectedRef)

	config := "terraform {\n  source = \"" + source + "\"\n}\n"
	require.NoError(
		t,
		os.WriteFile(filepath.Join(liveDir, "terragrunt.hcl"), []byte(config), 0o644),
	)

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan --source-update --non-interactive --working-dir "+liveDir,
	)

	// The crafted ref is a real branch, so the source downloads and plans, but
	// the injected command must never run.
	require.NoError(t, err)
	assert.NoFileExists(t, marker)
}
