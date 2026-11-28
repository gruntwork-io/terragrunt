package cli_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	authProviderCmd         = "auth-provider"
	authProviderAccessKeyID = "fake-access-key-id"

	authProviderRootAndGlobs = `"root": "repo", "globs": ["dev/vpc", "dev/app"]`
)

var authProviderCacheRoot = venvtest.Root("/repo")

// TestAuthProviderCacheSharedAcrossRunWithRacing pins that every credential fetch of one run --all, from discovery
// through dependency outputs to the units' own runs, looks up one response cache by the unit directory: the auth
// provider command fetches credentials once, and every tofu command runs with them.
func TestAuthProviderCacheSharedAcrossRunWithRacing(t *testing.T) {
	t.Parallel()

	var (
		mu           sync.Mutex
		keyed        int
		tofuCommands []tfCommand
		tofuCreds    []string
	)

	v := venv.MemVenv().
		WithFS(venvtest.NewFS(t, authProviderCacheRoot, map[string]string{
			".git/HEAD":              "ref: refs/heads/main\n",
			"dev/vpc/terragrunt.hcl": "",
			"dev/vpc/main.tf":        "",
			"dev/app/terragrunt.hcl": `dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  foo = dependency.vpc.outputs.foo
}
`,
			"dev/app/main.tf": "",
		})).
		WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			env := venv.ParseEnviron(inv.Env)

			mu.Lock()
			defer mu.Unlock()

			if filepath.Base(inv.Name) == authProviderCmd {
				if _, ok := env[externalcmd.EnvCacheKey]; !ok {
					return vexec.Result{Stdout: []byte(`{"cache": {` + authProviderRootAndGlobs + `, "ttl": -1}}`)}
				}

				keyed++

				return vexec.Result{Stdout: []byte(`{"awsCredentials": {"ACCESS_KEY_ID": "` + authProviderAccessKeyID +
					`", "SECRET_ACCESS_KEY": "fake-secret-access-key"}, "cache": {` + authProviderRootAndGlobs + `, "ttl": 60}}`)}
			}

			cmd := tfCommand{name: inv.Name, dir: inv.Dir, args: slices.Clone(inv.Args)}
			if isTFVersionProbe(cmd) {
				return vexec.Result{Stdout: []byte(tfBinaryVersions[tofuBinary])}
			}

			tofuCommands = append(tofuCommands, cmd)
			tofuCreds = append(tofuCreds, env["AWS_ACCESS_KEY_ID"])

			if len(inv.Args) > 0 && inv.Args[0] == "output" {
				return vexec.Result{Stdout: []byte(`{"foo":{"sensitive":false,"type":"string","value":"bar"}}`)}
			}

			return vexec.Result{}
		})

	_, err := runCLI(t, v,
		"run", "--all", "--non-interactive",
		"--working-dir", authProviderCacheRoot,
		"--experiment", "auth-provider-cache",
		"--auth-provider-cmd", authProviderCmd,
		"--", "plan",
	)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, 1, keyed)

	var planned []string

	for i, cmd := range tofuCommands {
		assert.Equal(t, authProviderAccessKeyID, tofuCreds[i], "tofu %v in %s ran without the cached credentials",
			cmd.args, cmd.dir)

		if len(cmd.args) == 0 || cmd.args[0] != "plan" {
			continue
		}

		for _, unit := range []string{"dev/vpc", "dev/app"} {
			if isUnder(cmd.dir, filepath.Join(authProviderCacheRoot, filepath.FromSlash(unit))) {
				planned = append(planned, unit)
			}
		}
	}

	assert.ElementsMatch(t, []string{"dev/vpc", "dev/app"}, planned)
}

// TestAuthProviderCacheReachedFromEveryCommand pins that every command that accepts --auth-provider-cmd has a case,
// and that each one fetching credentials takes the cached path, under the response cache RunAction installs,
// without panicking.
func TestAuthProviderCacheReachedFromEveryCommand(t *testing.T) {
	t.Parallel()

	unit := filepath.Join(authProviderCacheRoot, "dev", "vpc")
	stack := filepath.Join(authProviderCacheRoot, "stack")

	shortcuts := []string{
		"apply", "destroy", "force-unlock", "import", "init", "output", "plan", "refresh", "show", "state", "test",
		"validate",
	}

	testCases := make([]authProviderCommandCase, 0, len(shortcuts))

	for _, shortcut := range shortcuts {
		testCases = append(testCases, authProviderCommandCase{
			name:         shortcut,
			args:         []string{shortcut, "--working-dir", unit},
			fetchesCreds: true,
		})
	}

	testCases = append(testCases, []authProviderCommandCase{
		{name: "run", args: []string{"run", "--working-dir", unit}, tfArgs: []string{"plan"}, fetchesCreds: true},
		{
			name:         "run --all",
			args:         []string{"run", "--all", "--working-dir", authProviderCacheRoot},
			tfArgs:       []string{"plan"},
			fetchesCreds: true,
		},
		{
			name:         "run --graph",
			args:         []string{"run", "--graph", "--working-dir", unit},
			tfArgs:       []string{"plan"},
			fetchesCreds: true,
		},
		{name: "exec", args: []string{"exec", "--working-dir", unit}, tfArgs: []string{"echo"}, fetchesCreds: true},
		{name: "render", args: []string{"render", "--working-dir", unit}, fetchesCreds: true},
		{name: "info print", args: []string{"info", "print", "--working-dir", unit}, fetchesCreds: true},
		{
			name:         "aws-provider-patch",
			args:         []string{"aws-provider-patch", "--override-attr", "region=eu-west-1", "--working-dir", unit},
			fetchesCreds: true,
		},
		{name: "stack generate", args: []string{"stack", "generate", "--working-dir", stack}},
		{
			name:         "stack run",
			args:         []string{"stack", "run", "--working-dir", stack},
			tfArgs:       []string{"plan"},
			fetchesCreds: true,
		},
		{name: "stack output", args: []string{"stack", "output", "--working-dir", stack}, fetchesCreds: true},
	}...)

	require.ElementsMatch(t, authProviderCmdCommands(t), casesCommands(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cached atomic.Int32

			v := venv.MemVenv().
				WithFS(venvtest.NewFS(t, authProviderCacheRoot, map[string]string{
					".git/HEAD":              "ref: refs/heads/main\n",
					"dev/vpc/terragrunt.hcl": "",
					"dev/vpc/main.tf":        "",
					"stack/terragrunt.stack.hcl": `unit "vpc" {
  source = "../dev/vpc"
  path   = "vpc"
}
`,
					"stack/.terragrunt-stack/vpc/terragrunt.hcl": "",
					"stack/.terragrunt-stack/vpc/main.tf":        "",
				})).
				WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
					if filepath.Base(inv.Name) == authProviderCmd {
						if venv.ParseEnviron(inv.Env)[externalcmd.EnvSchema] == externalcmd.SchemaVersion {
							cached.Add(1)
						}

						return vexec.Result{Stdout: []byte(`{"awsCredentials": {"ACCESS_KEY_ID": "` +
							authProviderAccessKeyID + `", "SECRET_ACCESS_KEY": "fake-secret-access-key"}, ` +
							`"cache": {"root": "repo", "globs": ["**"], "ttl": 60}}`)}
					}

					if isTFVersionProbe(tfCommand{args: inv.Args}) {
						return vexec.Result{Stdout: []byte(tfBinaryVersions[tofuBinary])}
					}

					if len(inv.Args) > 0 && inv.Args[0] == "output" {
						return vexec.Result{Stdout: []byte(`{}`)}
					}

					return vexec.Result{}
				})

			args := append(slices.Clone(tc.args),
				"--non-interactive",
				"--experiment", "auth-provider-cache",
				"--auth-provider-cmd", authProviderCmd,
			)

			if tc.tfArgs != nil {
				args = append(append(args, "--"), tc.tfArgs...)
			}

			_, err := runCLI(t, v, args...)
			require.NoError(t, err)

			assert.Equal(t, tc.fetchesCreds, cached.Load() > 0)
		})
	}
}

// authProviderCommandCase is a CLI invocation that TestAuthProviderCacheReachedFromEveryCommand runs.
type authProviderCommandCase struct {
	name         string
	args         []string
	tfArgs       []string
	fetchesCreds bool
}

// authProviderCmdCommands returns the path of every CLI command that accepts --auth-provider-cmd, such as
// "stack run".
func authProviderCmdCommands(t *testing.T) []string {
	t.Helper()

	var paths []string

	var walk func(prefix []string, cmds clihelper.Commands)

	walk = func(prefix []string, cmds clihelper.Commands) {
		for _, cmd := range cmds {
			path := append(slices.Clone(prefix), cmd.Name)

			if cmd.Flags.Get(shared.AuthProviderCmdFlagName) != nil {
				paths = append(paths, strings.Join(path, " "))
			}

			walk(path, cmd.Subcommands)
		}
	}

	walk(nil, cli.NewApp(logger.CreateLogger(), options.NewTerragruntOptions(), venv.MemVenv()).Commands)

	return paths
}

// casesCommands returns the distinct command paths that testCases run: the arguments before the first flag.
func casesCommands(testCases []authProviderCommandCase) []string {
	paths := make([]string, 0, len(testCases))

	for _, tc := range testCases {
		end := slices.IndexFunc(tc.args, func(arg string) bool { return strings.HasPrefix(arg, "-") })
		paths = append(paths, strings.Join(tc.args[:end], " "))
	}

	slices.Sort(paths)

	return slices.Compact(paths)
}
