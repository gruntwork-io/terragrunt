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

	"github.com/gruntwork-io/terragrunt/internal/venv"

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

const slowGenerateDelay = 31 * time.Second

// TestGenerateStackProgress checks that a slow unit or stack is drawn and logs one done line next to its generate line.
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
		name          string
		wantInfoLines int
		reporter      bool
	}{
		{name: "reporter on the context", reporter: true, wantInfoLines: 2 * len(components)},
		{name: "plain context", wantInfoLines: len(components)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				targetDir := filepath.Join(enabledStackDir, config.StackDir)
				stackPath := filepath.Join(enabledStackDir, config.DefaultStackFile)

				v := venv.MemVenv().WithFS(slowDirFS{
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
				pctx.NoCAS = true

				term := new(bytes.Buffer)

				if tc.reporter {
					ctx = spinner.ContextWithReporter(ctx, drawingReporter(term))
				}

				logs := new(bytes.Buffer)
				l := logger.CreateLogger()
				l.SetOptions(log.WithOutput(util.NewSyncWriter(logs)), log.WithLevel(log.InfoLevel))

				pool := worker.NewWorkerPool(1)
				pool.Start()

				defer pool.Stop()

				require.NoError(t, config.GenerateStackFile(ctx, l, v, pctx, pool, stackPath))
				require.NoError(t, pool.Wait())

				for _, component := range components {
					assert.True(
						t,
						vfs.Exists(v.FS, filepath.Join(targetDir, component.name, component.config)),
						"the %s must be generated", component.kind,
					)
				}

				assert.Equal(t, tc.reporter, term.Len() > 0)
				assert.Equal(t, tc.wantInfoLines, strings.Count(logs.String(), "level=info"), logs)
			})
		})
	}
}

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
