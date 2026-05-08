package getter_test

import (
	"context"
	"net/url"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultClientRegistersS3GCS pins the protocol set NewClient registers,
// guarding against regressions where getters can be dropped silently.
func TestDefaultClientRegistersS3GCS(t *testing.T) {
	t.Parallel()

	client := getter.NewClient()

	assert.True(t, hasGetter[*getter.S3Getter](client.Getters), "default client must include S3Getter")
	assert.True(t, hasGetter[*getter.GCSGetter](client.Getters), "default client must include GCSGetter")
}

// TestDefaultClientCoversCanonicalProtocols pins the rest of the canonical
// Terragrunt protocol set so a future refactor can't silently drop one.
func TestDefaultClientCoversCanonicalProtocols(t *testing.T) {
	t.Parallel()

	client := getter.NewClient()

	assert.True(t, hasGetter[*getter.GitGetter](client.Getters), "git")
	assert.True(t, hasGetter[*getter.HgGetter](client.Getters), "hg")
	assert.True(t, hasGetter[*getter.HTTPSchemeGetter](client.Getters), "http(s)")
	assert.True(t, hasGetter[*getter.SmbClientGetter](client.Getters), "smb-client")
	assert.True(t, hasGetter[*getter.SmbMountGetter](client.Getters), "smb-mount")
	assert.True(t, hasGetter[*getter.FileGetter](client.Getters), "file")
}

// TestWithFileCopyReplacesFileGetter confirms WithFileCopy substitutes the
// stock FileGetter with FileCopyGetter (and only that one).
func TestWithFileCopyReplacesFileGetter(t *testing.T) {
	t.Parallel()

	client := getter.NewClient(getter.WithFileCopy(getter.NewFileCopyGetter()))

	assert.True(t, hasGetter[*getter.FileCopyGetter](client.Getters), "FileCopyGetter must be registered")
	assert.False(t, hasGetter[*getter.FileGetter](client.Getters), "stock FileGetter must be replaced")
}

// TestForcedGettersRouteToTheirGetter pins the routing for the v1->v2 forced
// prefixes Terragrunt cares about. The s3 and gcs cases back the v1.0.4
// changelog promise that `s3::` and `gcs::` stack sources now download.
//
// Routing is asserted by stubbing each forced prefix with a recording getter
// prepended to the chain, then calling Client.Get with the matching URL.
func TestForcedGettersRouteToTheirGetter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		forced string
		src    string
	}{
		{name: "s3 forced prefix", forced: "s3", src: "s3::https://s3.amazonaws.com/bucket/object"},
		{name: "gcs forced prefix", forced: "gcs", src: "gcs::https://www.googleapis.com/storage/v1/bucket/object"},
		{name: "git forced prefix", forced: "git", src: "git::https://github.com/foo/bar.git"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := &forcedRecordingGetter{forced: tc.forced}
			client := getter.NewClient(getter.WithCustomGettersPrepended(stub))

			_, err := client.Get(t.Context(), &getter.Request{
				Src:     tc.src,
				Dst:     t.TempDir(),
				GetMode: getter.ModeDir,
			})
			require.NoError(t, err)
			assert.Equal(t, 1, stub.calls, "stub must be invoked exactly once for %s", tc.forced)
		})
	}
}

func hasGetter[T any](getters []getter.Getter) bool {
	return slices.ContainsFunc(getters, func(g getter.Getter) bool {
		_, ok := g.(T)
		return ok
	})
}

// forcedRecordingGetter is a Getter that only matches a specific forced
// prefix. Tests use it to assert which getter the client picks for a given
// URL.
type forcedRecordingGetter struct {
	forced string
	calls  int
}

func (g *forcedRecordingGetter) Get(_ context.Context, _ *getter.Request) error {
	g.calls++
	return nil
}

func (g *forcedRecordingGetter) GetFile(_ context.Context, _ *getter.Request) error {
	g.calls++
	return nil
}

func (g *forcedRecordingGetter) Mode(_ context.Context, _ *url.URL) (getter.Mode, error) {
	return getter.ModeDir, nil
}

func (g *forcedRecordingGetter) Detect(req *getter.Request) (bool, error) {
	return req.Forced == g.forced, nil
}
