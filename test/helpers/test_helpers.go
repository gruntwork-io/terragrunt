package helpers

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/stretchr/testify/require"
)

const defaultDirPerms = 0755

func IsWindows() bool {
	return runtime.GOOS == "windows"
}

func ValidateHookTraceParent(t *testing.T, hook, str string) {
	t.Helper()

	traceparentLine := ""

	for line := range strings.SplitSeq(str, "\n") {
		if strings.HasPrefix(line, hook+" {\"traceparent\": \"") {
			traceparentLine = line
			break
		}
	}

	require.NotEmpty(t, traceparentLine, "Expected "+hook+" output with traceparent value")
	re := regexp.MustCompile(hook + ` \{"traceparent": "([^"]+)"\}`)
	matches := re.FindStringSubmatch(traceparentLine)

	const matchesCount = 2

	require.Len(t, matches, matchesCount, "Expected to extract traceparent value from hook output")

	traceparentValue := matches[1]
	require.NotEmpty(t, traceparentValue, "Traceparent value should not be empty")
	require.Regexp(t, `^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`, traceparentValue, "Traceparent value should match W3C traceparent format")
}

// CreateFile creates an empty file at the given path, creating parent directories if needed.
func CreateFile(t *testing.T, paths ...string) {
	t.Helper()

	fullPath := filepath.Join(paths...)

	err := os.MkdirAll(filepath.Dir(fullPath), defaultDirPerms)
	require.NoError(t, err)

	f, err := os.Create(fullPath)
	require.NoError(t, err)

	err = f.Close()
	require.NoError(t, err)
}

// IsRunnerPoolExperimentEnabled returns true if either TG_EXPERIMENT_MODE is set or TG_EXPERIMENT is set to "runner-pool".
func IsRunnerPoolExperimentEnabled(t *testing.T) bool {
	t.Helper()
	return IsExperimentMode(t) || os.Getenv("TG_EXPERIMENT") == experiment.RunnerPool
}

// IsExperimentMode returns true if the TG_EXPERIMENT_MODE environment variable is set.
func IsExperimentMode(t *testing.T) bool {
	t.Helper()
	// check if TG_EXPERIMENT_MODE is set in env
	return os.Getenv("TG_EXPERIMENT_MODE") != ""
}
