package vfs_test

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

func TestDigestFileMatchesDigestOfTheSameContents(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(vfs.MemRoot, "file")
	content := bytes.Repeat([]byte("0123456789"), 10_000)
	require.NoError(t, vfs.WriteFile(fsys, path, content, 0o644))

	digest, err := vfs.DigestFile(fsys, path)
	require.NoError(t, err)

	assert.Equal(t, vfs.Digest(content), digest)
	assert.NotEqual(t, vfs.Digest(content[1:]), digest)
}

func TestIsRacyModTime(t *testing.T) {
	t.Parallel()

	readAt := time.Now()

	tcs := []struct {
		modTime time.Time
		name    string
		want    bool
	}{
		{name: "an hour before the read", modTime: readAt.Add(-time.Hour), want: false},
		{name: "a second before the read", modTime: readAt.Add(-time.Second), want: true},
		{name: "after the read", modTime: readAt.Add(time.Second), want: true},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, vfs.IsRacyModTime(tc.modTime, readAt))
		})
	}
}
