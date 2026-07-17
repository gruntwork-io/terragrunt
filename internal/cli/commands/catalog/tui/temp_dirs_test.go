package tui_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTempDirTrackerCleanupRemovesTrackedDirs(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "catalog-cleanup")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	tracker := tui.NewTempDirTracker(vfs.NewOSFS())
	tracker.Track(dir)
	tracker.Cleanup(logger.CreateLogger())

	_, err := os.Stat(dir)
	assert.True(t, os.IsNotExist(err), "tracked catalog temp dir should be removed")

	tracker.Cleanup(logger.CreateLogger())
}
