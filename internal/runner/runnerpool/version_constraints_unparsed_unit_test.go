package runnerpool_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// TestCheckVersionConstraints_UnparsedUnit_ResolvesConfigPathAgainstUnitDir is a
// regression test for a unit whose Config() is still nil when
// checkVersionConstraints's concurrent pass reaches it. This happens in
// practice for a dependency discovered only as a lightweight placeholder by
// the relationship-discovery phase (internal/discovery/phase_relationship.go's
// dependencyToDiscover, via component.NewUnit) rather than through the main
// filesystem-discovery walk, which is the only path that eagerly parses a
// unit's config.
//
// unit.ConfigFile() returns a bare basename by design (component.NewUnit's own
// default, and discovery.createComponentFromPath's unit.SetConfigFile(base)) —
// every other caller in this codebase joins it with unit.Path() before using
// it as a file path. checkUnitVersionConstraints's fallback parse must do the
// same, or PartialParseConfigFile's os.Stat resolves the bare name against the
// process's cwd instead of the unit's own directory and fails with
// TerragruntConfigNotFoundError even though the file genuinely exists on disk.
func TestCheckVersionConstraints_UnparsedUnit_ResolvesConfigPathAgainstUnitDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	unitDir := filepath.Join(tmpDir, "some-unit")
	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(unitDir, "terragrunt.hcl"),
		[]byte("terraform {\n  source = \"./modules/noop\"\n}\n"),
		0644,
	))

	// Mirrors exactly what a dependency reached only via the
	// relationship-discovery phase's placeholder path looks like: Path() set
	// to the unit's real directory, ConfigFile() left at its bare-basename
	// default ("terragrunt.hcl"), Config() never populated.
	unit := component.NewUnit(unitDir)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(unitDir, "terragrunt.hcl"))
	require.NoError(t, err)

	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)
	l := log.New(log.WithFormatter(formatter))

	err = runnerpool.CheckVersionConstraints(t.Context(), l, venv.OSVenv(), opts, []*component.Unit{unit})
	require.NoError(t, err, "checkVersionConstraints must resolve the unit's real config path (unit.Path() joined with unit.ConfigFile()), not a bare basename resolved against the process's cwd")
}
