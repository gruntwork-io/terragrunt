package rcfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/rcfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const jsonConfig = `{
  "env": {
    "TG_PROVIDER_CACHE": "1"
  },
  "flags": [
    { "name": "non-interactive", "default": true },
    { "name": "log-level", "default": "debug" }
  ],
  "commands": [
    { "name": "run", "flags": [ { "name": "queue-include-dir", "default": ["a", "b"] } ] }
  ]
}`

const yamlConfig = `
env:
  TG_PROVIDER_CACHE: "1"
flags:
  - name: non-interactive
    default: true
  - name: log-level
    default: debug
commands:
  - name: run
    flags:
      - name: queue-include-dir
        default:
          - a
          - b
`

func TestLoad(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		fileName string
		content  string
	}{
		{name: "json", fileName: rcfile.BaseName + ".json", content: jsonConfig},
		{name: "yaml", fileName: rcfile.BaseName + ".yaml", content: yamlConfig},
		{name: "yml", fileName: rcfile.BaseName + ".yml", content: yamlConfig},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, t.TempDir(), tc.fileName, tc.content)

			cfg, err := rcfile.Load(path)
			require.NoError(t, err)
			require.NotNil(t, cfg)

			assert.Equal(t, path, cfg.Path)
			assert.Equal(t, map[string]string{"TG_PROVIDER_CACHE": "1"}, cfg.EnvVars())

			values, ok := cfg.FlagValues(nil, []string{"non-interactive"})
			assert.True(t, ok)
			assert.Equal(t, []string{"true"}, values)

			values, ok = cfg.FlagValues(nil, []string{"log-level"})
			assert.True(t, ok)
			assert.Equal(t, []string{"debug"}, values)

			values, ok = cfg.FlagValues([][]string{{"run"}}, []string{"queue-include-dir"})
			assert.True(t, ok)
			assert.Equal(t, []string{"a", "b"}, values)
		})
	}
}

func TestLoadEmptyFile(t *testing.T) {
	t.Parallel()

	for _, fileName := range rcfile.FileNames() {
		t.Run(fileName, func(t *testing.T) {
			t.Parallel()

			cfg, err := rcfile.Load(writeFile(t, t.TempDir(), fileName, "  \n"))
			require.NoError(t, err)
			require.NotNil(t, cfg)

			assert.Empty(t, cfg.Flags)
			assert.Empty(t, cfg.Commands)
			assert.Empty(t, cfg.EnvVars())
		})
	}
}

func TestLoadNumericAndListDefaults(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		fileName string
		content  string
		expected []string
	}{
		{
			name:     "json whole number",
			fileName: rcfile.BaseName + ".json",
			content:  `{"flags": [{"name": "parallelism", "default": 8}]}`,
			expected: []string{"8"},
		},
		{
			name:     "yaml whole number",
			fileName: rcfile.BaseName + ".yaml",
			content:  "flags:\n  - name: parallelism\n    default: 8\n",
			expected: []string{"8"},
		},
		{
			name:     "json fractional number",
			fileName: rcfile.BaseName + ".json",
			content:  `{"flags": [{"name": "parallelism", "default": 1.5}]}`,
			expected: []string{"1.5"},
		},
		{
			name:     "json list of scalars",
			fileName: rcfile.BaseName + ".json",
			content:  `{"flags": [{"name": "experiment", "default": ["stacks", true, 2]}]}`,
			expected: []string{"stacks", "true", "2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := rcfile.Load(writeFile(t, t.TempDir(), tc.fileName, tc.content))
			require.NoError(t, err)

			values, ok := cfg.FlagValues(nil, []string{"parallelism", "experiment"})
			assert.True(t, ok)
			assert.Equal(t, tc.expected, values)
		})
	}
}

func TestLoadInvalid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		expectedErr error
		name        string
		fileName    string
		content     string
	}{
		{
			name:     "unknown json field",
			fileName: rcfile.BaseName + ".json",
			content:  `{"flag": [{"name": "non-interactive", "default": true}]}`,
		},
		{
			name:     "unknown yaml field",
			fileName: rcfile.BaseName + ".yaml",
			content:  "flag:\n  - name: non-interactive\n",
		},
		{
			name:     "malformed json",
			fileName: rcfile.BaseName + ".json",
			content:  `{"flags": [`,
		},
		{
			name:        "flag without a name",
			fileName:    rcfile.BaseName + ".json",
			content:     `{"flags": [{"default": true}]}`,
			expectedErr: rcfile.ErrMissingFlagName,
		},
		{
			name:        "flag without a default",
			fileName:    rcfile.BaseName + ".json",
			content:     `{"flags": [{"name": "non-interactive"}]}`,
			expectedErr: rcfile.ErrMissingFlagDefault,
		},
		{
			name:        "unsupported default",
			fileName:    rcfile.BaseName + ".json",
			content:     `{"flags": [{"name": "non-interactive", "default": {"a": 1}}]}`,
			expectedErr: rcfile.ErrUnsupportedFlagDefault,
		},
		{
			name:        "command without a name",
			fileName:    rcfile.BaseName + ".json",
			content:     `{"commands": [{"flags": [{"name": "all", "default": true}]}]}`,
			expectedErr: rcfile.ErrMissingCommandName,
		},
		{
			name:        "command flag without a name",
			fileName:    rcfile.BaseName + ".json",
			content:     `{"commands": [{"name": "run", "flags": [{"default": true}]}]}`,
			expectedErr: rcfile.ErrMissingFlagName,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := rcfile.Load(writeFile(t, t.TempDir(), tc.fileName, tc.content))
			require.Error(t, err)
			assert.Nil(t, cfg)

			if tc.expectedErr != nil {
				assert.ErrorIs(t, err, tc.expectedErr)
			}
		})
	}
}

func TestConfigFlagValues(t *testing.T) {
	t.Parallel()

	content := `{
	  "flags": [{"name": "non-interactive", "default": true}],
	  "commands": [
	    {"name": "run", "flags": [{"name": "all", "default": true}]},
	    {"name": "hcl fmt", "flags": [{"name": "diff", "default": true}]}
	  ]
	}`

	cfg, err := rcfile.Load(writeFile(t, t.TempDir(), rcfile.BaseName+".json", content))
	require.NoError(t, err)

	testCases := []struct {
		name          string
		cmdPath       [][]string
		flagNames     []string
		expected      []string
		expectedFound bool
	}{
		{
			name:          "global flag",
			flagNames:     []string{"non-interactive"},
			expected:      []string{"true"},
			expectedFound: true,
		},
		{
			name:      "global flag is not applied to a command",
			cmdPath:   [][]string{{"run"}},
			flagNames: []string{"non-interactive"},
		},
		{
			name:          "command flag by command name",
			cmdPath:       [][]string{{"run"}},
			flagNames:     []string{"all"},
			expected:      []string{"true"},
			expectedFound: true,
		},
		{
			name:          "nested command flag by full path",
			cmdPath:       [][]string{{"hcl"}, {"format", "fmt"}},
			flagNames:     []string{"diff"},
			expected:      []string{"true"},
			expectedFound: true,
		},
		{
			name:          "flag matched by an alias",
			flagNames:     []string{"alias", "non-interactive"},
			expected:      []string{"true"},
			expectedFound: true,
		},
		{
			name:          "command flag by own name",
			cmdPath:       [][]string{{"stack"}, {"run"}},
			flagNames:     []string{"all"},
			expected:      []string{"true"},
			expectedFound: true,
		},
		{
			name:      "command flag is not applied to another command",
			cmdPath:   [][]string{{"stack"}},
			flagNames: []string{"all"},
		},
		{
			name:      "full path is not matched by a shorter path",
			cmdPath:   [][]string{{"format", "fmt"}},
			flagNames: []string{"diff"},
		},
		{
			name:      "undeclared flag",
			flagNames: []string{"log-level"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			values, ok := cfg.FlagValues(tc.cmdPath, tc.flagNames)
			assert.Equal(t, tc.expectedFound, ok)
			assert.Equal(t, tc.expected, values)
		})
	}
}

func TestConfigFlagValuesNilConfig(t *testing.T) {
	t.Parallel()

	var cfg *rcfile.Config

	values, ok := cfg.FlagValues(nil, []string{"non-interactive"})
	assert.False(t, ok)
	assert.Nil(t, values)
	assert.Nil(t, cfg.EnvVars())
}

//nolint:paralleltest // mutates the environment to make expansion deterministic.
func TestConfigEnvVars(t *testing.T) {
	t.Setenv("TG_RCFILE_TEST_VALUE", "expanded")

	dir := t.TempDir()
	content := `{
	  "env": {
	    "PLAIN": "value",
	    "EXPANDED": "$TG_RCFILE_TEST_VALUE",
	    "BRACED": "${TG_RCFILE_TEST_VALUE}/sub",
	    "RELATIVE": "./terraformrc.hcl",
	    "PARENT": "../shared/terraformrc.hcl",
	    "ABSOLUTE": "/etc/terraformrc.hcl",
	    "URL": "https://registry.example.com/v1/providers/"
	  }
	}`

	cfg, err := rcfile.Load(writeFile(t, dir, rcfile.BaseName+".json", content))
	require.NoError(t, err)

	expected := map[string]string{
		"PLAIN":    "value",
		"EXPANDED": "expanded",
		"BRACED":   "expanded/sub",
		"RELATIVE": filepath.Join(dir, "terraformrc.hcl"),
		"PARENT":   filepath.Join(filepath.Dir(dir), "shared", "terraformrc.hcl"),
		"ABSOLUTE": "/etc/terraformrc.hcl",
		"URL":      "https://registry.example.com/v1/providers/",
	}

	assert.Equal(t, expected, cfg.EnvVars())
}

//nolint:paralleltest // relies on a home directory that only this test may change.
func TestFind(t *testing.T) {
	root := isolatedHome(t)

	repoRoot := filepath.Join(root, "repo")
	unitDir := filepath.Join(repoRoot, "envs", "prod")
	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(repoRoot, ".git"), 0755))

	// Nothing to find yet.
	cfg, err := rcfile.Find(unitDir)
	require.NoError(t, err)
	assert.Nil(t, cfg)

	// The home directory is the last resort.
	homePath := writeFile(t, root, rcfile.BaseName+".json", `{"flags": [{"name": "a", "default": "home"}]}`)
	assertFound(t, unitDir, homePath)

	// The .config directory at the repository root wins over the home directory.
	configDir := filepath.Join(repoRoot, rcfile.ConfigDirName)
	require.NoError(t, os.Mkdir(configDir, 0755))
	configPath := writeFile(t, configDir, rcfile.BaseName+".json", `{"flags": [{"name": "a", "default": "config"}]}`)
	assertFound(t, unitDir, configPath)

	// The repository root wins over its .config directory.
	repoPath := writeFile(t, repoRoot, rcfile.BaseName+".yaml", "flags:\n  - name: a\n    default: repo\n")
	assertFound(t, unitDir, repoPath)

	// The working directory wins over every parent.
	unitPath := writeFile(t, unitDir, rcfile.BaseName+".yml", "flags:\n  - name: a\n    default: unit\n")
	assertFound(t, unitDir, unitPath)

	// JSON wins over YAML within the same directory.
	unitJSONPath := writeFile(t, unitDir, rcfile.BaseName+".json", `{"flags": [{"name": "a", "default": "unit json"}]}`)
	assertFound(t, unitDir, unitJSONPath)
}

//nolint:paralleltest // relies on a home directory that only this test may change.
func TestFindStopsAtRepoRoot(t *testing.T) {
	root := isolatedHome(t)

	outside := filepath.Join(root, "outside")
	repoRoot := filepath.Join(outside, "repo")
	require.NoError(t, os.MkdirAll(repoRoot, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(repoRoot, ".git"), 0755))

	writeFile(t, outside, rcfile.BaseName+".json", `{"flags": [{"name": "a", "default": "outside"}]}`)

	cfg, err := rcfile.Find(repoRoot)
	require.NoError(t, err)
	assert.Nil(t, cfg, "a directory above the repository root must not configure the run")
}

//nolint:paralleltest // relies on a home directory that only this test may change.
func TestFindWithoutRepo(t *testing.T) {
	root := isolatedHome(t)

	nested := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0755))

	writeFile(t, filepath.Join(root, "a"), rcfile.BaseName+".json", `{"flags": [{"name": "a", "default": "parent"}]}`)

	cfg, err := rcfile.Find(nested)
	require.NoError(t, err)
	assert.Nil(t, cfg, "outside a repository only the working directory is searched")
}

//nolint:paralleltest // relies on a home directory that only this test may change.
func TestSearchDirs(t *testing.T) {
	root := isolatedHome(t)

	repoRoot := filepath.Join(root, "repo")
	unitDir := filepath.Join(repoRoot, "envs", "prod")
	require.NoError(t, os.MkdirAll(unitDir, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(repoRoot, ".git"), 0755))

	dirs, err := rcfile.SearchDirs(unitDir)
	require.NoError(t, err)

	expected := []string{
		unitDir,
		filepath.Join(repoRoot, "envs"),
		repoRoot,
		filepath.Join(repoRoot, rcfile.ConfigDirName),
		filepath.Join(root, ".config", rcfile.AppDirName),
		root,
	}

	assert.Equal(t, expected, dirs)
}

// assertFound checks that a search from startDir returns the config stored at path.
func assertFound(t *testing.T, startDir, path string) {
	t.Helper()

	cfg, err := rcfile.Find(startDir)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, path, cfg.Path)
}

// isolatedHome points the home and user configuration directories at a new temporary
// directory, so that discovery cannot reach a real configuration file, and returns it.
func isolatedHome(t *testing.T) string {
	t.Helper()

	root := t.TempDir()

	// t.TempDir can hand out a path through a symlink, for example /var on macOS, while
	// discovery works with the resolved path.
	resolved, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)

	t.Setenv("HOME", resolved)
	t.Setenv("USERPROFILE", resolved)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(resolved, ".config"))

	return resolved
}

// writeFile writes content to dir/name and returns the path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	return path
}
