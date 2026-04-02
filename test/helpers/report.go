package helpers

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/stretchr/testify/require"
)

// ReadReport reads and parses a JSON report file, failing the test if it cannot be read.
func ReadReport(t *testing.T, rootPath string, reportFile string) report.JSONRuns {
	t.Helper()

	runs, err := report.ParseJSONRunsFromFile(filepath.Join(rootPath, reportFile))
	require.NoError(t, err)

	return runs
}
