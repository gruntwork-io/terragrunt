package report_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportEndOptionsSetRunFields(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	r := report.NewReport()

	path := filepath.Join(t.TempDir(), "unit")

	run, err := r.EnsureRun(l, path,
		report.WithCauseRunError("boom"),
		report.WithRef("v1.2.3"),
		report.WithCmd("plan"),
		report.WithArgs([]string{"-no-color", "-input=false"}),
	)
	require.NoError(t, err)

	require.NotNil(t, run.Cause)
	assert.Equal(t, report.Cause("boom"), *run.Cause)
	assert.Equal(t, "v1.2.3", run.Ref)
	assert.Equal(t, "plan", run.Cmd)
	assert.Equal(t, []string{"-no-color", "-input=false"}, run.Args)
}

func TestReportSortRuns(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	r := report.NewReport()

	dir := t.TempDir()

	first, err := r.EnsureRun(l, filepath.Join(dir, "first"))
	require.NoError(t, err)

	second, err := r.EnsureRun(l, filepath.Join(dir, "second"))
	require.NoError(t, err)

	// Force out-of-order start times so SortRuns has something to reorder.
	base := time.Now()
	first.Started = base.Add(time.Hour)
	second.Started = base

	r.SortRuns()

	require.Len(t, r.Runs, 2)
	assert.Equal(t, second.Path, r.Runs[0].Path, "earlier Started sorts first")
	assert.Equal(t, first.Path, r.Runs[1].Path)
}

func TestReportWriteToFile(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	dir := "/reports"

	t.Run("json format", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		require.NoError(t, fsys.MkdirAll(dir, 0o755))

		r := report.NewReport().WithFormat(report.FormatJSON)
		_, err := r.EnsureRun(l, filepath.Join(dir, "json-unit"))
		require.NoError(t, err)

		out := filepath.Join(dir, "report.json")
		require.NoError(t, r.WriteToFile(fsys, out))

		raw, err := vfs.ReadFile(fsys, out)
		require.NoError(t, err)
		assert.Contains(t, string(raw), "json-unit")
	})

	t.Run("unsupported format errors", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()

		r := report.NewReport()
		_, err := r.EnsureRun(l, filepath.Join(dir, "no-format-unit"))
		require.NoError(t, err)

		require.Error(t, r.WriteToFile(fsys, filepath.Join(dir, "unused.out")),
			"writing with no format set is rejected")
	})
}
