package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadStackConfigFilePropagatesAutoIncludeStatError pins that a filesystem fault
// while probing for the stack autoinclude file aborts the parse. Reading the fault as
// "no autoinclude file here" would silently drop an autoinclude the user wrote and
// generate the stack without it.
func TestReadStackConfigFilePropagatesAutoIncludeStatError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	stackPath := filepath.Join(tmpDir, config.DefaultStackFile)
	require.NoError(t, vfs.WriteFile(vfs.NewOSFS(), stackPath, []byte(`
unit "app" {
  source = "./units/app"
  path   = "app"
}
`), 0644))

	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv().WithFS(statErrorFS{
		FS:       vfs.NewOSFS(),
		failPath: filepath.Join(tmpDir, inthclparse.AutoIncludeStackFile),
	}), stackPath)

	_, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), pctx, stackPath, nil)
	require.ErrorIs(t, err, errStatFailed)
}

// TestReadValuesPropagatesStatError pins that a filesystem fault while probing for a
// unit's values file surfaces to the caller. The absent case returns a nil value and a
// nil error, so folding the fault into it would hand back a unit with no values and
// nothing to indicate anything went wrong.
func TestReadValuesPropagatesStatError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	ctx, pctx := newTestParsingContext(
		t,
		venvtest.NewOSWithEmptyEnv().WithFS(statErrorFS{
			FS:       vfs.NewOSFS(),
			failPath: filepath.Join(tmpDir, "terragrunt.values.hcl"),
		}),
		filepath.Join(tmpDir, config.DefaultTerragruntConfigPath),
	)

	values, err := config.ReadValues(ctx, pctx, logger.CreateLogger(), tmpDir)
	require.ErrorIs(t, err, errStatFailed)
	assert.Nil(t, values)
}

// errStatFailed stands in for what a filesystem reports when it cannot answer a
// question about a path, such as a directory it may not read or a path component that
// turned out to be a file.
var errStatFailed = errors.New("stat failed")

// statErrorFS fails Stat for one exact path and delegates every other call, so a test
// can tell apart a file that is absent from one whose presence cannot be determined.
type statErrorFS struct {
	vfs.FS
	failPath string
}

func (fsys statErrorFS) Stat(name string) (os.FileInfo, error) {
	if name == fsys.failPath {
		return nil, &os.PathError{Op: "stat", Path: name, Err: errStatFailed}
	}

	return fsys.FS.Stat(name)
}
