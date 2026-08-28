package cli_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/require"
)

const unitRoot = "/units"

// runCLI drives the whole CLI against v and returns what the run wrote to
// standard output. Colors are off so output compares byte for byte.
func runCLI(t *testing.T, v *venv.Venv, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer

	v = v.WithWriter(&out)

	l := logger.CreateLogger()
	l.Formatter().SetDisabledColors(true)

	err := cli.NewApp(l, options.NewTerragruntOptions(v.Exec), v).
		Run(l, v, append([]string{"terragrunt"}, args...))

	return out.String(), err
}

// oneUnit returns a venv holding a single unit, for the runs that need
// somewhere to point --working-dir but never get far enough to read it.
func oneUnit(t *testing.T) *venv.Venv {
	t.Helper()

	return venvtest.New().WithFS(venvtest.NewFS(t, unitRoot, map[string]string{
		"terragrunt.hcl": "",
	}))
}

const discoveryRoot = "/components"

// runDiscovery runs a discovery command against files and returns what it
// printed. Callers compare whole strings, which pins ordering and column
// widths along with membership.
func runDiscovery(t *testing.T, files map[string]string, args ...string) string {
	t.Helper()

	v := venvtest.New().WithFS(venvtest.NewFS(t, discoveryRoot, files))

	out, err := runCLI(t, v, append(args, "--no-color", "--working-dir", discoveryRoot)...)
	require.NoError(t, err)

	return out
}

func runFind(t *testing.T, files map[string]string, args ...string) string {
	t.Helper()

	return runDiscovery(t, files, append([]string{"find"}, args...)...)
}

func runList(t *testing.T, files map[string]string, args ...string) string {
	t.Helper()

	return runDiscovery(t, files, append([]string{"list"}, args...)...)
}
