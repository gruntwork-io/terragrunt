package config

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
)

// Pins that only a data dir inside the working dir can vouch for that working dir being initialized.
//
// A TF_DATA_DIR resolving outside the working dir is shared by every unit that inherits the variable,
// so its existence says nothing about this dependency. Before this was enforced, one shared directory
// made every dependency look initialized and outputs were read from a cache dir that never existed,
// which surfaced as a chdir failure on plan and as silently empty outputs on render.
func TestDependencyInitDataDir(t *testing.T) {
	t.Parallel()

	workingDir := filepath.Join(string(filepath.Separator), "tmp", "cache", "unit")
	defaultDataDir := filepath.Join(workingDir, tf.DefaultTFDataDir)

	tcs := []struct {
		name    string
		dataDir string
		want    string
	}{
		{
			name:    "unset falls back to the default inside the working dir",
			dataDir: "",
			want:    defaultDataDir,
		},
		{
			name:    "relative inside the working dir is honoured",
			dataDir: ".tf_data",
			want:    filepath.Join(workingDir, ".tf_data"),
		},
		{
			name:    "nested relative inside the working dir is honoured",
			dataDir: filepath.Join("nested", ".tf_data"),
			want:    filepath.Join(workingDir, "nested", ".tf_data"),
		},
		{
			name:    "absolute outside the working dir falls back",
			dataDir: filepath.Join(string(filepath.Separator), "shared", "tf-data"),
			want:    defaultDataDir,
		},
		{
			name:    "relative climbing out of the working dir falls back",
			dataDir: filepath.Join("..", "shared"),
			want:    defaultDataDir,
		},
		{
			name:    "equal to the working dir falls back",
			dataDir: workingDir,
			want:    defaultDataDir,
		},
		{
			name:    "absolute but inside the working dir is honoured",
			dataDir: filepath.Join(workingDir, ".tf_data"),
			want:    filepath.Join(workingDir, ".tf_data"),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			env := map[string]string{}
			if tc.dataDir != "" {
				env["TF_DATA_DIR"] = tc.dataDir
			}

			pctx := &ParsingContext{Venv: venvtest.NewOSWithEmptyEnv().WithEnv(env)}

			assert.Equal(t, tc.want, dependencyInitDataDir(pctx, workingDir))
		})
	}
}

// Pins that dependencyStateDataDir still returns an absolute TF_DATA_DIR verbatim for workspace resolution.
func TestDependencyStateDataDirKeepsAbsolutePath(t *testing.T) {
	t.Parallel()

	workingDir := filepath.Join(string(filepath.Separator), "tmp", "cache", "unit")
	shared := filepath.Join(string(filepath.Separator), "shared", "tf-data")

	pctx := &ParsingContext{
		Venv: venvtest.NewOSWithEmptyEnv().WithEnv(map[string]string{"TF_DATA_DIR": shared}),
	}

	assert.Equal(t, shared, dependencyStateDataDir(pctx, workingDir),
		"workspace resolution must keep honouring an absolute TF_DATA_DIR, since that is where tofu writes it")
}
