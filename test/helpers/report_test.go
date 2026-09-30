package helpers_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadReport(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	reason := "run error"

	want := report.JSONRuns{
		{
			Name:    "live/app",
			Result:  "succeeded",
			Started: started,
			Ended:   started.Add(time.Second),
		},
		{
			Name:    "live/db",
			Result:  "failed",
			Reason:  &reason,
			Started: started,
			Ended:   started.Add(2 * time.Second),
		},
	}

	data, err := json.Marshal(want)
	require.NoError(t, err)

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "report.json"), data, 0o600))

	assert.Equal(t, want, helpers.ReadReport(t, root, "report.json"))
}
