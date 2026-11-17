package mcp_test

import (
	"path/filepath"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// moduleVariables declares one required and one optional input.
const moduleVariables = `
variable "name" {
  type = string
}

variable "cidr" {
  type    = string
  default = "10.0.0.0/16"
}
`

// unitValuesConfig is a catalog unit whose inputs come from values.
const unitValuesConfig = `
inputs = {
  name = values.name
}
`

type scaffoldOutput struct {
	OutputFolder string   `json:"output_folder"`
	Kind         string   `json:"kind"`
	Files        []string `json:"files"`
	TODO         []string `json:"todo"`
	Degraded     []string `json:"degraded"`
	FileCount    int      `json:"file_count"`
}

// newModuleTree writes a tree holding one module under modules/vpc.
func newModuleTree(t *testing.T) string {
	t.Helper()

	return writeTree(t, map[string]string{"modules/vpc/variables.tf": moduleVariables})
}

// newScaffoldSession is [newTestSession] with a real temporary directory to
// download sources into.
func newScaffoldSession(t *testing.T, dir string) *mcp.ClientSession {
	t.Helper()

	tempDir := t.TempDir()
	v := venvtest.NewWithOSFS().WithTempDir(func() string { return tempDir })

	return newTestSessionWith(t, dir, v, func(*tgmcp.Options) {}, nil)
}

// callToolRefused calls the named tool and requires it to fail with want as
// the cause.
func callToolRefused(
	t *testing.T,
	session *mcp.ClientSession,
	name string,
	args map[string]any,
	want error,
) {
	t.Helper()

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.True(t, res.IsError, "the %s tool call must be refused", name)
	require.Len(t, res.Content, 1)

	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok, "the refusal must be text: %T", res.Content[0])
	assert.Contains(t, text.Text, want.Error())
}

func readTreeFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := vfs.ReadFile(vfs.NewOSFS(), path)
	require.NoError(t, err)

	return string(contents)
}

// TestScaffoldGeneratesAModuleWithTheValuesGiven pins that output_folder
// defaults to working_dir, and that a relative source is read against it.
func TestScaffoldGeneratesAModuleWithTheValuesGiven(t *testing.T) {
	t.Parallel()

	dir := newModuleTree(t)
	require.NoError(t, vfs.NewOSFS().MkdirAll(filepath.Join(dir, "live", "vpc"), 0o755))

	session := newScaffoldSession(t, dir)

	var out scaffoldOutput

	callTool(t, session, "scaffold", map[string]any{
		"source":      "../../modules/vpc",
		"working_dir": "live/vpc",
		"values":      map[string]any{"name": `"main"`},
	}, &out)

	assert.Equal(t, scaffoldOutput{
		OutputFolder: filepath.Join("live", "vpc"),
		Kind:         "module",
		Files:        []string{"terragrunt.hcl"},
		FileCount:    1,
	}, out)

	generated := readTreeFile(t, filepath.Join(dir, "live", "vpc", "terragrunt.hcl"))
	assert.Contains(t, generated, `source = "../../modules/vpc"`)
	assert.Contains(t, generated, `name = "main"`)
}

func TestScaffoldListsTheRequiredValuesLeftOut(t *testing.T) {
	t.Parallel()

	dir := newModuleTree(t)
	session := newScaffoldSession(t, dir)

	var out scaffoldOutput

	callTool(t, session, "scaffold", map[string]any{
		"source":        "modules/vpc",
		"output_folder": "live/vpc",
	}, &out)

	assert.Equal(t, []string{"name"}, out.TODO)
	assert.Contains(
		t,
		readTreeFile(t, filepath.Join(dir, "live", "vpc", "terragrunt.hcl")),
		"# TODO: fill in value",
	)
}

func TestScaffoldPassesVarsToTheTemplate(t *testing.T) {
	t.Parallel()

	dir := writeTree(t, map[string]string{
		"modules/vpc/variables.tf":      moduleVariables,
		"templates/env/boilerplate.yml": "variables:\n  - name: Env\n    type: string\n",
		"templates/env/terragrunt.hcl":  "# env = {{ .Env }}\n",
		"vars/staging.yml":              "Env: from-file\n",
	})
	session := newScaffoldSession(t, dir)

	var fromVars scaffoldOutput

	callTool(t, session, "scaffold", map[string]any{
		"source":        "modules/vpc",
		"template":      filepath.ToSlash(filepath.Join(dir, "templates", "env")),
		"output_folder": "live/from-vars",
		"vars":          map[string]any{"Env": "from-vars"},
	}, &fromVars)

	assert.Contains(
		t,
		readTreeFile(t, filepath.Join(dir, "live", "from-vars", "terragrunt.hcl")),
		"# env = from-vars",
	)

	var fromFile scaffoldOutput

	callTool(t, session, "scaffold", map[string]any{
		"source":        "modules/vpc",
		"template":      filepath.ToSlash(filepath.Join(dir, "templates", "env")),
		"output_folder": "live/from-file",
		"var_files":     []any{"vars/staging.yml"},
	}, &fromFile)

	assert.Contains(
		t,
		readTreeFile(t, filepath.Join(dir, "live", "from-file", "terragrunt.hcl")),
		"# env = from-file",
	)
}

func TestScaffoldCopiesAUnitWithItsValues(t *testing.T) {
	t.Parallel()

	dir := writeTree(t, map[string]string{"units/app/terragrunt.hcl": unitValuesConfig})
	session := newScaffoldSession(t, dir)

	var out scaffoldOutput

	callTool(t, session, "scaffold", map[string]any{
		"source":        "units/app",
		"output_folder": "live/app",
		"values":        map[string]any{"name": `"app"`},
	}, &out)

	assert.Equal(t, "unit", out.Kind)
	assert.Equal(t, []string{"terragrunt.hcl", "terragrunt.values.hcl"}, out.Files)
	assert.Empty(t, out.TODO)
	assert.Equal(
		t,
		unitValuesConfig,
		readTreeFile(t, filepath.Join(dir, "live", "app", "terragrunt.hcl")),
	)
	assert.Contains(
		t,
		readTreeFile(t, filepath.Join(dir, "live", "app", "terragrunt.values.hcl")),
		`name = "app"`,
	)
}

// TestScaffoldOutputFolderIsRelativeToWorkingDir pins that output_folder and
// a relative source are both read against working_dir, as the scaffold
// command reads --output-folder and a source against --working-dir.
func TestScaffoldOutputFolderIsRelativeToWorkingDir(t *testing.T) {
	t.Parallel()

	dir := newModuleTree(t)
	require.NoError(t, vfs.NewOSFS().MkdirAll(filepath.Join(dir, "live"), 0o755))

	session := newScaffoldSession(t, dir)

	var out scaffoldOutput

	callTool(t, session, "scaffold", map[string]any{
		"source":        "../modules/vpc",
		"working_dir":   "live",
		"output_folder": "vpc",
	}, &out)

	assert.Equal(t, filepath.Join("live", "vpc"), out.OutputFolder)
	assert.Equal(t, []string{"terragrunt.hcl"}, out.Files)
	assert.FileExists(t, filepath.Join(dir, "live", "vpc", "terragrunt.hcl"))
	assert.NoDirExists(t, filepath.Join(dir, "modules", "vpc", "vpc"))
}

// TestScaffoldWritesIntoAnEmptyOutputFolder pins that an existing directory is
// accepted as long as there is nothing in it to overwrite.
func TestScaffoldWritesIntoAnEmptyOutputFolder(t *testing.T) {
	t.Parallel()

	dir := newModuleTree(t)
	require.NoError(t, vfs.NewOSFS().MkdirAll(filepath.Join(dir, "live", "vpc"), 0o755))

	session := newScaffoldSession(t, dir)

	var out scaffoldOutput

	callTool(t, session, "scaffold", map[string]any{
		"source":        "modules/vpc",
		"output_folder": "live/vpc",
	}, &out)

	assert.Equal(t, []string{"terragrunt.hcl"}, out.Files)
}

func TestScaffoldRefusesAnOutputFolderThatIsNotEmpty(t *testing.T) {
	t.Parallel()

	dir := writeTree(t, map[string]string{
		"modules/vpc/variables.tf": moduleVariables,
		"live/vpc/terragrunt.hcl":  "# keep\n",
	})
	session := newScaffoldSession(t, dir)

	callToolRefused(t, session, "scaffold", map[string]any{
		"source":        "modules/vpc",
		"output_folder": "live/vpc",
	}, tgmcp.ErrScaffoldOutputFolderNotEmpty)

	assert.Equal(
		t,
		"# keep\n",
		readTreeFile(t, filepath.Join(dir, "live", "vpc", "terragrunt.hcl")),
	)
}

func TestScaffoldOutputFolderIsConfinedToTheServerRoot(t *testing.T) {
	t.Parallel()

	parent := writeTree(t, map[string]string{"root/modules/vpc/variables.tf": moduleVariables})
	dir := filepath.Join(parent, "root")
	outside := writeTree(t, map[string]string{})
	require.NoError(t, vfs.Symlink(vfs.NewOSFS(), outside, filepath.Join(dir, "link")))

	session := newScaffoldSession(t, dir)

	for _, outputDir := range []string{"../outside", "link/vpc", filepath.ToSlash(outside)} {
		callToolRefused(t, session, "scaffold", map[string]any{
			"source":        filepath.ToSlash(filepath.Join(dir, "modules", "vpc")),
			"output_folder": outputDir,
		}, tgmcp.ErrOutsideRoot)
	}

	assert.NoDirExists(t, filepath.Join(parent, "outside"))
	assert.NoDirExists(t, filepath.Join(outside, "vpc"))
}

// TestScaffoldRefusesTemplatesThatReachPastTheOutputFolder pins the templates
// the tool will not hand to boilerplate, and that a refused call leaves no
// output folder behind.
func TestScaffoldRefusesTemplatesThatReachPastTheOutputFolder(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		files map[string]string
		name  string
	}{
		{
			name: "templated file name",
			files: map[string]string{
				"boilerplate.yml":           "variables:\n  - name: Dir\n    type: string\n    default: x\n",
				"{{ .Dir }}/terragrunt.hcl": "\n",
			},
		},
		{
			name: "URL-encoded templated file name",
			files: map[string]string{
				"boilerplate.yml":             "variables:\n  - name: Dir\n    type: string\n    default: x\n",
				"%7B%7B .Dir %7D%7D/main.hcl": "\n",
			},
		},
		{
			name: "dependency",
			files: map[string]string{
				"boilerplate.yml": "dependencies:\n  - name: other\n    template-url: ../other\n    output-folder: ../../outside\n",
				"terragrunt.hcl":  "\n",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{"modules/vpc/variables.tf": moduleVariables}
			for name, contents := range tc.files {
				files["modules/vpc/.boilerplate/"+name] = contents
			}

			dir := writeTree(t, files)
			session := newScaffoldSession(t, dir)

			callToolRefused(t, session, "scaffold", map[string]any{
				"source":        "modules/vpc",
				"output_folder": "live/vpc",
			}, tgmcp.ErrScaffoldUnsafeTemplate)

			assert.NoDirExists(t, filepath.Join(dir, "live"))
		})
	}
}

// TestScaffoldDoesNotRunTemplateShellHelpers pins that a template's shell
// helper does not run, even under --allow=exec.
func TestScaffoldDoesNotRunTemplateShellHelpers(t *testing.T) {
	t.Parallel()

	dir := newModuleTree(t)
	marker := filepath.Join(dir, "ran")

	template := writeTree(t, map[string]string{
		"boilerplate.yml": "variables: []\n",
		"terragrunt.hcl":  `# {{ shell "touch" "` + filepath.ToSlash(marker) + `" }}` + "\n",
	})

	session := newRealExecSession(t, dir, func(o *tgmcp.Options) { o.Allow = []string{"exec"} })

	var out scaffoldOutput

	callTool(t, session, "scaffold", map[string]any{
		"source":        "modules/vpc",
		"template":      filepath.ToSlash(template),
		"output_folder": "live/vpc",
	}, &out)

	assert.NoFileExists(t, marker)
}

func TestScaffoldRefusesInvalidValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		values map[string]any
		want   error
		name   string
	}{
		{
			name:   "not an expression",
			values: map[string]any{"name": "\"main\"\n}\nlocals {"},
			want:   tgmcp.ErrScaffoldValueNotHCL,
		},
		{
			name:   "empty",
			values: map[string]any{"name": ""},
			want:   tgmcp.ErrScaffoldValueNotHCL,
		},
		{
			name:   "name the module does not take",
			values: map[string]any{"region": `"us-east-1"`},
			want:   tgmcp.ErrScaffoldUnknownValue,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := newModuleTree(t)
			session := newScaffoldSession(t, dir)

			callToolRefused(t, session, "scaffold", map[string]any{
				"source":        "modules/vpc",
				"output_folder": "live/vpc",
				"values":        tc.values,
			}, tc.want)

			assert.NoDirExists(t, filepath.Join(dir, "live"))
		})
	}
}

// TestScaffoldRefusesARemoteSourceWithoutTheCapability pins that a source
// download goes through the server's network and exec gates.
func TestScaffoldRefusesARemoteSourceWithoutTheCapability(t *testing.T) {
	t.Parallel()

	dir := newModuleTree(t)
	session := newScaffoldSession(t, dir)

	for source, want := range map[string]error{
		"https://example.invalid/modules/vpc.zip":                      vhttp.ErrNoNetwork,
		"git::https://example.invalid/org/modules.git//vpc?ref=v1.0.0": venv.ErrExecDenied,
	} {
		callToolRefused(t, session, "scaffold", map[string]any{
			"source":        source,
			"output_folder": "live/vpc",
		}, want)
	}

	assert.NoDirExists(t, filepath.Join(dir, "live"))
}

func TestScaffoldVarFilesAreConfinedToTheServerRoot(t *testing.T) {
	t.Parallel()

	dir := newModuleTree(t)
	outside := writeTree(t, map[string]string{"vars.yml": "Env: outside\n"})
	session := newScaffoldSession(t, dir)

	callToolRefused(t, session, "scaffold", map[string]any{
		"source":        "modules/vpc",
		"output_folder": "live/vpc",
		"var_files":     []any{filepath.ToSlash(filepath.Join(outside, "vars.yml"))},
	}, tgmcp.ErrOutsideRoot)

	assert.NoDirExists(t, filepath.Join(dir, "live"))
}

// TestScaffoldRefusesModuleArgumentsForAUnit pins that a unit, which is
// copied as written, does not quietly drop a template or vars it was given.
func TestScaffoldRefusesModuleArgumentsForAUnit(t *testing.T) {
	t.Parallel()

	dir := writeTree(t, map[string]string{"units/app/terragrunt.hcl": unitValuesConfig})
	session := newScaffoldSession(t, dir)

	callToolRefused(t, session, "scaffold", map[string]any{
		"source":        "units/app",
		"output_folder": "live/app",
		"vars":          map[string]any{"Env": "prod"},
	}, tgmcp.ErrScaffoldNotAModule)

	assert.NoDirExists(t, filepath.Join(dir, "live"))
}

func TestScaffoldRefusesInvalidArguments(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		args map[string]any
		want error
		name string
	}{
		{
			name: "empty source",
			args: map[string]any{"source": "", "output_folder": "live/vpc"},
			want: tgmcp.ErrScaffoldSourceRequired,
		},
		{
			name: "vars name holding '='",
			args: map[string]any{
				"source":        "modules/vpc",
				"output_folder": "live/vpc",
				"vars":          map[string]any{"Env=prod": "x"},
			},
			want: tgmcp.ErrScaffoldVarName,
		},
		{
			name: "var_files entry that is a directory",
			args: map[string]any{
				"source":        "modules/vpc",
				"output_folder": "live/vpc",
				"var_files":     []any{"modules"},
			},
			want: tgmcp.ErrScaffoldVarFileNotFile,
		},
		{
			name: "output_folder that is a file",
			args: map[string]any{
				"source":        "modules/vpc",
				"output_folder": "modules/vpc/variables.tf",
			},
			want: tgmcp.ErrScaffoldOutputFolderNotDir,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := newModuleTree(t)
			session := newScaffoldSession(t, dir)

			callToolRefused(t, session, "scaffold", tc.args, tc.want)

			assert.NoDirExists(t, filepath.Join(dir, "live"))
		})
	}
}
