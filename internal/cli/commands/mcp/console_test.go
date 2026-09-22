package mcp_test

import (
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTofu stands in for the binary --tf-path names. It answers the version
// check and `output -json`, so a test can tell which binary a tool started
// without installing one.
const fakeTofu = "#!/bin/sh\n" +
	"case \"$1\" in\n" +
	"  -version|version) echo 'OpenTofu v1.9.0' ;;\n" +
	"  output) echo '{\"id\":{\"sensitive\":false,\"type\":\"string\",\"value\":\"from-tf-path\"}}' ;;\n" +
	"esac\n"

// runTerragrunt runs args through the CLI app the way the terragrunt binary
// does, with stdin and stdout on the streams given.
func runTerragrunt(t *testing.T, stdin io.Reader, stdout io.Writer, args ...string) error {
	t.Helper()

	v := venvtest.NewOSWithEmptyEnv().WithStdin(stdin).WithWriter(stdout).WithErrWriter(io.Discard)
	l := logger.CreateLogger()

	return cli.NewApp(l, options.NewTerragruntOptions(v.Exec), v).
		RunContext(t.Context(), l, v, slices.Concat([]string{"terragrunt"}, args))
}

// mcpArgs returns the arguments that run the mcp command with its experiment
// enabled, followed by args.
func mcpArgs(args ...string) []string {
	return slices.Concat([]string{tgmcp.CommandName, "--experiment", experiment.MCPCommand}, args)
}

// TestMCPCommandServesOnStdio pins the command as the binary runs it. Its flags
// reach the server, the protocol runs over stdin and stdout, a tool starts the
// binary --tf-path names, and a client disconnecting ends the command cleanly.
func TestMCPCommandServesOnStdio(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}

	tfPath := filepath.Join(t.TempDir(), "tofu-from-flag")
	require.NoError(t, vfs.WriteFile(vfs.NewOSFS(), tfPath, []byte(fakeTofu), 0o755))

	dir := writeTree(t, map[string]string{
		"a/terragrunt.hcl": "\n",
		"a/main.tf":        "\n",
	})

	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()

	served := make(chan error, 1)

	go func() {
		err := runTerragrunt(t, serverIn, serverOut,
			mcpArgs("--working-dir", dir, "--allow=exec", "--tf-path", tfPath)...)
		served <- errors.Join(err, serverIn.CloseWithError(err), serverOut.CloseWithError(err))
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "terragrunt-test", Version: "1"}, nil)

	session, err := client.Connect(t.Context(), &mcp.IOTransport{Reader: clientIn, Writer: clientOut}, nil)
	require.NoError(t, err)

	var out getOutputsOutput

	callTool(t, session, "get_outputs", map[string]any{"working_dir": "a"}, &out)
	assert.Equal(t, map[string]outputEntry{"id": {Value: "from-tf-path"}}, out.Outputs)

	require.NoError(t, session.Close())

	select {
	case err := <-served:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		require.Fail(t, "the mcp command did not return after the client disconnected")
	}
}

// TestMCPCommandRefusesFlagsItCannotHonor pins the checks the command makes on
// its flags, as the CLI parses them, before it serves anything.
func TestMCPCommandRefusesFlagsItCannotHonor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		want error
		name string
		args []string
	}{
		{
			name: "without the experiment",
			args: []string{tgmcp.CommandName},
			want: tgmcp.ErrExperimentRequired,
		},
		{
			name: "apply without exec",
			args: mcpArgs("--dangerously-allow-apply"),
			want: tgmcp.ErrApplyRequiresExec,
		},
		{
			name: "allow-cmd without exec",
			args: mcpArgs("--allow-cmd", "jq **"),
			want: tgmcp.ErrAllowCmdRequiresExec,
		},
		{
			name: "a pattern with a shell operator",
			args: mcpArgs("--allow=exec", "--allow-cmd", "jq | sh"),
			want: tgmcp.ErrInvalidAllowCmd,
		},
		{
			name: "an unknown capability",
			args: mcpArgs("--allow=root"),
			want: tgmcp.ErrUnknownCapability,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			args := slices.Concat(tc.args, []string{"--working-dir", t.TempDir()})

			require.ErrorIs(t, runTerragrunt(t, strings.NewReader(""), io.Discard, args...), tc.want)
		})
	}
}

// TestMCPCommandIsolatesTheProcessEnvironment pins that the command replaces
// the process environment before serving, keeping PATH under --allow=exec and
// dropping credentials without --allow=env.
func TestMCPCommandIsolatesTheProcessEnvironment(t *testing.T) {
	t.Parallel()

	var replaced map[string]string

	v := venvtest.NewOSWithEmptyEnv().
		WithEnv(map[string]string{"PATH": "/usr/bin", "AWS_ACCESS_KEY_ID": "AKIAEXAMPLE"}).
		WithReplaceEnviron(func(env map[string]string) error {
			replaced = env
			return nil
		}).
		WithStdin(strings.NewReader("")).
		WithWriter(io.Discard).
		WithErrWriter(io.Discard)
	l := logger.CreateLogger()

	args := slices.Concat([]string{"terragrunt"}, mcpArgs("--working-dir", t.TempDir(), "--allow=exec"))
	require.NoError(t, cli.NewApp(l, options.NewTerragruntOptions(v.Exec), v).RunContext(t.Context(), l, v, args))

	assert.Equal(t, "/usr/bin", replaced["PATH"])
	assert.NotContains(t, replaced, "AWS_ACCESS_KEY_ID")
	assert.NotEmpty(t, replaced["HOME"])
	assert.NoDirExists(t, replaced["HOME"], "the empty home should be removed once the server stops")
}

// TestMCPCommandRefusesToServeWhenTheEnvironmentCannotBeReplaced pins that a
// failed replacement stops the command and removes the empty directories it
// made for HOME and PATH.
func TestMCPCommandRefusesToServeWhenTheEnvironmentCannotBeReplaced(t *testing.T) {
	t.Parallel()

	errReplace := errors.New("replace failed")

	var home string

	v := venvtest.NewOSWithEmptyEnv().
		WithReplaceEnviron(func(env map[string]string) error {
			home = env["HOME"]
			return errReplace
		}).
		WithStdin(strings.NewReader("")).
		WithWriter(io.Discard).
		WithErrWriter(io.Discard)
	l := logger.CreateLogger()

	args := slices.Concat([]string{"terragrunt"}, mcpArgs("--working-dir", t.TempDir()))
	err := cli.NewApp(l, options.NewTerragruntOptions(v.Exec), v).RunContext(t.Context(), l, v, args)

	require.ErrorIs(t, err, errReplace)
	require.NotEmpty(t, home)
	assert.NoDirExists(t, filepath.Dir(home))
}
