//go:build !windows

package cas_test

import (
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// TestCASGetterRefOptionInjection checks the CAS git source path never runs a command from a crafted ref.
func TestCASGetterRefOptionInjection(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	marker := filepath.Join(helpers.TmpDirWOSymlinks(t), "injected")

	// ${IFS} avoids a literal space, which git rejects in a ref name.
	injectedRef := "--upload-pack=touch${IFS}" + marker

	repoDir := helpers.TmpDirWOSymlinks(t)
	helpers.CreateFile(t, repoDir, "main.tf")
	helpers.InitGitRepoWithBranchRef(t, repoDir, injectedRef)

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venv.OSVenv()

	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &cas.CloneOptions{Depth: 1})
	client := getter.Client{Getters: []getter.Getter{g}}

	src := "git::file://" + repoDir + "?ref=" + url.QueryEscape(injectedRef)

	dst := helpers.TmpDirWOSymlinks(t)

	_, err = client.Get(ctx, &getter.Request{Src: src, Dst: dst})

	// The crafted ref is a real branch, so the download still succeeds and
	// materializes the module, but the injected command must never run.
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dst, "main.tf"))
	assert.NoFileExists(t, marker)
}
