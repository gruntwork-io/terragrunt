package cli_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInfoPrintAllWritesJSONLines pins that a run over several units writes
// one unit per line, so a consumer can parse the output a line at a time.
func TestInfoPrintAllWritesJSONLines(t *testing.T) {
	t.Parallel()

	out, err := runCLI(t, infoPrintUnits(t), "info", "print", "--all", "--working-dir", unitRoot)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2)

	paths := make([]string, 0, len(lines))

	for _, line := range lines {
		var info struct {
			ConfigPath string `json:"config_path"`
		}

		require.NoError(t, json.Unmarshal([]byte(line), &info))

		paths = append(paths, info.ConfigPath)
	}

	assert.Equal(t, []string{
		filepath.Join(unitRoot, "a", "terragrunt.hcl"),
		filepath.Join(unitRoot, "b", "terragrunt.hcl"),
	}, paths)
}

// TestInfoPrintWritesIndentedJSON pins that a single unit stays pretty
// printed, which is what a reader of one unit's info gets.
func TestInfoPrintWritesIndentedJSON(t *testing.T) {
	t.Parallel()

	out, err := runCLI(t, infoPrintUnits(t), "info", "print", "--working-dir", filepath.Join(unitRoot, "a"))
	require.NoError(t, err)

	assert.Contains(t, out, "\n  \"config_path\":")

	var info map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &info))
}

// TestInfoPrintAllReportsEachUnitsContext pins the object a unit gets: its
// own config and download directory, the ones `run --all` gives it, and a
// working directory inside that download directory, where its sources land.
func TestInfoPrintAllReportsEachUnitsContext(t *testing.T) {
	t.Parallel()

	out, err := runCLI(t, infoPrintUnits(t), "info", "print", "--all", "--working-dir", unitRoot)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2)

	for i, name := range []string{"a", "b"} {
		var info print.InfoOutput

		require.NoError(t, json.Unmarshal([]byte(lines[i]), &info))

		unitDir := filepath.Join(unitRoot, name)
		downloadDir := filepath.Join(unitDir, util.TerragruntCacheDir)

		assert.Equal(t, filepath.Join(unitDir, config.DefaultTerragruntConfigPath), info.ConfigPath)
		assert.Equal(t, downloadDir, info.DownloadDir)
		assert.Empty(t, info.IAMRole)
		assert.Equal(t, options.TofuDefaultPath, info.TerraformBinary)
		assert.Equal(t, "print", info.TerraformCommand)
		assert.True(
			t,
			strings.HasPrefix(info.WorkingDir, downloadDir+string(filepath.Separator)),
			"working dir %q must sit inside the unit's download dir %q",
			info.WorkingDir,
			downloadDir,
		)
	}
}

func infoPrintUnits(t *testing.T) *venv.Venv {
	t.Helper()

	return venvtest.New().WithFS(venvtest.NewFS(t, unitRoot, map[string]string{
		"a/terragrunt.hcl": "",
		"b/terragrunt.hcl": "",
	}))
}
