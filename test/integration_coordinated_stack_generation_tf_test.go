//go:build tf

package test_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTFStackRunSharesGenerationAcrossInitialization(t *testing.T) {
	t.Parallel()

	f := newDependencyStackFixture(t, true)
	for range 2 {
		_, stderr, err := f.run("producer", "stack run --source-update -- plan")
		require.NoError(t, err, "%s", stderr)
		require.Equal(t, 1, strings.Count(stderr, "Generating unit db from "), "%s", stderr)
	}
}
