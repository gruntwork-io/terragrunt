//go:build aws

package test_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	tggetter "github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// TestAwsCASS3ChecksumProbe exercises CASGetter end-to-end against a
// real S3 bucket. The PutObject sets a SHA-256 checksum so the
// resolver's preferred content-addressed path runs; on a second
// CASGetter request CAS materializes from the local store without
// re-downloading the archive.
func TestAwsCASS3ChecksumProbe(t *testing.T) {
	t.Parallel()

	region := helpers.TerraformRemoteStateS3Region
	key := "modules/example.tar.gz"
	bucket := provisionS3ModuleArchive(t, region, key)

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venv.OSVenv()

	g := tggetter.NewCASGetter(
		logger.CreateLogger(),
		c,
		v,
		&tgcas.CloneOptions{},
		tggetter.WithDefaultGenericDispatch(),
	)
	client := &tggetter.Client{Getters: []tggetter.Getter{g}}

	src := "s3::https://s3-" + region + ".amazonaws.com/" + bucket + "/" + key

	first := filepath.Join(helpers.TmpDirWOSymlinks(t), "first")
	_, err = client.Get(t.Context(), &tggetter.Request{
		Src:     src,
		Dst:     first,
		GetMode: tggetter.ModeAny,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(first, "main.tf"))

	second := filepath.Join(helpers.TmpDirWOSymlinks(t), "second")
	_, err = client.Get(t.Context(), &tggetter.Request{
		Src:     src,
		Dst:     second,
		GetMode: tggetter.ModeAny,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(second, "main.tf"))
}
