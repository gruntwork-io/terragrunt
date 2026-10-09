package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowGenerateDelay is how long each component takes to fetch, one second past the first keepalive line.
const slowGenerateDelay = 31 * time.Second

// TestGenerateStackProgress pins that a slow unit or stack is reported without repeating the line that announces it.
func TestGenerateStackProgress(t *testing.T) {
	t.Parallel()

	components := []struct {
		kind   string
		name   string
		config string
	}{
		{kind: "unit", name: "app", config: config.DefaultTerragruntConfigPath},
		{kind: "stack", name: "team", config: config.DefaultStackFile},
	}

	testCases := []struct {
		name        string
		wantReports int
		reporter    bool
	}{
		{name: "reporter on the context", reporter: true, wantReports: 1},
		{name: "plain context"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				targetDir := filepath.Join(enabledStackDir, config.StackDir)
				stackPath := filepath.Join(enabledStackDir, config.DefaultStackFile)

				v := venvtest.New().WithFS(slowDirFS{
					FS: venvtest.NewFS(t, enabledStackDir, map[string]string{
						filepath.Join(enabledUnitSource, config.DefaultTerragruntConfigPath): "",
						filepath.Join(enabledStackSource, config.DefaultStackFile):           "",
						config.DefaultStackFile: `
unit "app" {
  source = "` + enabledUnitSource + `"
  path   = "app"
}

stack "team" {
  source = "` + enabledStackSource + `"
  path   = "team"
}
`,
					}),
					dirs:  []string{filepath.Join(targetDir, "app"), filepath.Join(targetDir, "team")},
					delay: slowGenerateDelay,
				})

				ctx, pctx := newTestParsingContext(t, stackPath)
				pctx.TerragruntStackConfigPath = stackPath
				// CAS shells out to git, which the no-spawn venv refuses.
				pctx.NoCAS = true

				if tc.reporter {
					ctx = spinner.ContextWithReporter(ctx, spinner.New(spinner.Options{}))
				}

				logs := new(bytes.Buffer)
				l := logger.CreateLogger()
				l.SetOptions(log.WithOutput(util.NewSyncWriter(logs)), log.WithLevel(log.InfoLevel))

				pool := worker.NewWorkerPool(1)
				pool.Start()

				defer pool.Stop()

				require.NoError(t, config.GenerateStackFile(ctx, l, v, pctx, pool, stackPath))
				// Drain the pool before reading the buffer the workers log into.
				require.NoError(t, pool.Wait())

				lines := strings.Split(logs.String(), "\n")

				for _, component := range components {
					announced := "Generating " + component.kind + " " + component.name +
						" from ." + string(filepath.Separator) + config.DefaultStackFile
					done := "Generated " + component.kind + " " + component.name + " (31.0s)"

					assert.True(
						t,
						vfs.Exists(v.FS, filepath.Join(targetDir, component.name, component.config)),
						"the %s must be generated", component.kind,
					)
					assert.Equal(t, 1, countSuffix(lines, announced), logs)
					assert.Zero(t, countSuffix(lines, announced+"..."), logs)
					assert.Equal(t, tc.wantReports, countSuffix(lines, announced+"... (30s elapsed)"), logs)
					assert.Equal(t, tc.wantReports, countSuffix(lines, done), logs)
				}
			})
		})
	}
}

// slowDirFS takes delay to create each of dirs and delegates every other call.
type slowDirFS struct {
	vfs.FS
	dirs  []string
	delay time.Duration
}

func (fsys slowDirFS) MkdirAll(path string, perm os.FileMode) error {
	if slices.Contains(fsys.dirs, path) && !vfs.Exists(fsys.FS, path) {
		time.Sleep(fsys.delay)
	}

	return fsys.FS.MkdirAll(path, perm)
}

// countSuffix returns how many of lines end with suffix.
func countSuffix(lines []string, suffix string) int {
	count := 0

	for _, line := range lines {
		if strings.HasSuffix(line, suffix) {
			count++
		}
	}

	return count
}
