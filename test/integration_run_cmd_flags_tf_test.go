//go:build tf

package test_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFRunCmdQuietRedactsOutput(t *testing.T) {
	t.Parallel()

	result := runCmdFlagsFixture(t)

	assert.Contains(t, result.stderr, "run_cmd output: [REDACTED]")
	assert.NotContains(t, result.stderr, runCmdSecretValue)
}

func TestTFRunCmdGlobalCacheSharesResultAcrossModules(t *testing.T) {
	t.Parallel()

	result := runCmdFlagsFixture(t)

	combinedOutput := strings.Join([]string{result.stdout, result.stderr}, "\n")

	globalCounterPath := filepath.Join(result.rootPath, "scripts", "global_counter.txt")
	globalCounterBytes, readErr := os.ReadFile(globalCounterPath)
	require.NoError(t, readErr)

	assert.Equal(t, "1", strings.TrimSpace(string(globalCounterBytes)))
	assert.Contains(t, combinedOutput, expectedGlobalCachedValue)
	assert.NotContains(t, combinedOutput, unexpectedGlobalCachedSecondValue)
}

func TestTFRunCmdNoCacheSkipsCachedValue(t *testing.T) {
	t.Parallel()

	result := runCmdFlagsFixture(t)

	assert.Contains(t, result.stderr, "run_cmd output: ["+expectedNoCacheFirstValue+"]")
	assert.NotContains(t, result.stderr, "run_cmd, cached output: ["+expectedNoCacheFirstValue+"]")
}
