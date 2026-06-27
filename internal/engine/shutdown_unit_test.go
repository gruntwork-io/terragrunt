package engine_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unitCacheKey mimics the engine registry key that tf.NewSource produces for a unit:
// filepath.Join(downloadDir, EncodeBase64Sha1(Clean(workingDir)), rootPath, modulePath).
func unitCacheKey(downloadDir, workingDir string) string {
	return filepath.Join(downloadDir, util.EncodeBase64Sha1(filepath.Clean(workingDir)), "github.com/acme/mod", ".")
}

func iacEngineEnabled(t *testing.T) experiment.Experiments {
	t.Helper()

	exps := experiment.NewExperiments()
	require.NoError(t, exps.EnableExperiment(experiment.IacEngine))

	return exps
}

// TestPathHasSegment verifies a unit's engine is identified by the hashed-workingDir
// segment of its cache key: matched independently of the download dir, isolated from
// other units, and never on a bare substring.
func TestPathHasSegment(t *testing.T) {
	t.Parallel()

	const unitA, unitB = "/repo/unitA", "/repo/unitB"

	segA := util.EncodeBase64Sha1(filepath.Clean(unitA))

	assert.True(t, engine.PathHasSegment(unitCacheKey("/tmp/dl", unitA), segA),
		"the target unit's cache key contains its own hash segment")
	assert.True(t, engine.PathHasSegment(unitCacheKey("/some/custom/download-dir", unitA), segA),
		"the match is download-dir-independent")
	assert.False(t, engine.PathHasSegment(unitCacheKey("/tmp/dl", unitB), segA),
		"another unit's cache key must not match unit A's hash segment")
	assert.False(t, engine.PathHasSegment("/a/foobar/c", "foo"),
		"a substring that is not a whole path component must not match")
}

// TestShutdownUnit_NoOpWhenGated asserts ShutdownUnit returns nil without touching the
// registry when the iac-engine experiment is disabled or NoEngine is set.
func TestShutdownUnit_NoOpWhenGated(t *testing.T) {
	t.Parallel()

	ctx := engine.WithEngineValues(context.Background())

	const unit = "/repo/unit"

	require.NoError(t, engine.ShutdownUnit(ctx, log.New(), experiment.NewExperiments(), false, unit),
		"a disabled iac-engine experiment must be a no-op")
	require.NoError(t, engine.ShutdownUnit(ctx, log.New(), iacEngineEnabled(t), true, unit),
		"NoEngine must be a no-op")
}
