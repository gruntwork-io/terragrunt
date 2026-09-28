package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// parseTrackingReads parses hcl against files and returns the paths the parse
// recorded as read.
func parseTrackingReads(t *testing.T, files map[string]string, hcl string) []string {
	t.Helper()

	l := logger.CreateLogger()
	v := venvtest.New().WithFS(memUnitFS(t, files))
	ctx, pctx := newTestParsingContext(t, memConfigPath)
	ctx = config.WithConfigValues(ctx)
	pctx.FilesRead = config.NewFilesRead()

	_, err := config.ParseConfigString(ctx, l, v, pctx, memConfigPath, hcl, nil)
	require.NoError(t, err)

	return pctx.FilesRead.Paths()
}

func TestHCLFileRecordsTheFileAsRead(t *testing.T) {
	t.Parallel()

	paths := parseTrackingReads(t,
		map[string]string{"data.txt": "contents\n"},
		`locals {
  data = file("data.txt")
}`,
	)

	assert.Contains(t, paths, filepath.Join(memUnitDir, "data.txt"))
}

func TestHCLFileHashRecordsTheFileAsRead(t *testing.T) {
	t.Parallel()

	paths := parseTrackingReads(t,
		map[string]string{"data.txt": "contents\n"},
		`locals {
  sum = filesha256("data.txt")
}`,
	)

	assert.Contains(t, paths, filepath.Join(memUnitDir, "data.txt"))
}

func TestHCLFileExistsRecordsTheFileAsRead(t *testing.T) {
	t.Parallel()

	paths := parseTrackingReads(t,
		map[string]string{"data.txt": "contents\n"},
		`locals {
  there = fileexists("data.txt")
}`,
	)

	assert.Contains(t, paths, filepath.Join(memUnitDir, "data.txt"))
}

func TestHCLFileExistsLeavesAMissingFileUnrecorded(t *testing.T) {
	t.Parallel()

	paths := parseTrackingReads(t,
		map[string]string{"data.txt": "contents\n"},
		`locals {
  there = fileexists("missing.txt")
}`,
	)

	assert.NotContains(t, paths, filepath.Join(memUnitDir, "missing.txt"))
}

func TestHCLTemplateFileRecordsTheTemplateAsRead(t *testing.T) {
	t.Parallel()

	paths := parseTrackingReads(t,
		map[string]string{"greeting.tmpl": "Hello, ${name}!"},
		`locals {
  greeting = templatefile("greeting.tmpl", { name = "world" })
}`,
	)

	assert.Contains(t, paths, filepath.Join(memUnitDir, "greeting.tmpl"))
}

func TestHCLFileSetRecordsEachMatchAsRead(t *testing.T) {
	t.Parallel()

	paths := parseTrackingReads(t,
		map[string]string{
			"a.tf":     "",
			"b.tf":     "",
			"skip.txt": "",
		},
		`locals {
  found = fileset(".", "*.tf")
}`,
	)

	assert.Contains(t, paths, filepath.Join(memUnitDir, "a.tf"))
	assert.Contains(t, paths, filepath.Join(memUnitDir, "b.tf"))
	assert.NotContains(t, paths, filepath.Join(memUnitDir, "skip.txt"))
}

func TestHCLFileReadsAreUnrecordedWithoutTracking(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New().WithFS(memUnitFS(t, map[string]string{"data.txt": "contents\n"}))
	ctx, pctx := newTestParsingContext(t, memConfigPath)
	ctx = config.WithConfigValues(ctx)

	const hcl = `locals {
  data = file("data.txt")
}`

	out, err := config.ParseConfigString(ctx, l, v, pctx, memConfigPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out.Locals)
	assert.Equal(t, "contents\n", out.Locals["data"])
	assert.Empty(t, pctx.FilesRead.Paths())
}

func TestHCLTemplateFileRecordsANestedFileAsRead(t *testing.T) {
	t.Parallel()

	paths := parseTrackingReads(t,
		map[string]string{
			"outer.tmpl": `${file("inner.txt")}`,
			"inner.txt":  "nested\n",
		},
		`locals {
  rendered = templatefile("outer.tmpl", {})
}`,
	)

	assert.Contains(t, paths, filepath.Join(memUnitDir, "outer.tmpl"))
	assert.Contains(t, paths, filepath.Join(memUnitDir, "inner.txt"))
}

func TestHCLTemplateFileRecordsANestedTemplateAsRead(t *testing.T) {
	t.Parallel()

	paths := parseTrackingReads(t,
		map[string]string{
			"outer.tmpl": `${templatefile("inner.tmpl", {})}`,
			"inner.tmpl": "nested",
		},
		`locals {
  rendered = templatefile("outer.tmpl", {})
}`,
	)

	assert.Contains(t, paths, filepath.Join(memUnitDir, "outer.tmpl"))
	assert.Contains(t, paths, filepath.Join(memUnitDir, "inner.tmpl"))
}
