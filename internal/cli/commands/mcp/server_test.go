package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

const unitAConfig = `
inputs = {
  name = "a"
}
`

const unitBConfig = `
dependency "a" {
  config_path = "../a"

  mock_outputs = {
    id = "mock-a"
  }
}

inputs = {
  a_id = dependency.a.outputs.id
}
`

// unitRunCmdConfig names a program in a local, so evaluating the unit is
// enough to reach the shell if anything is willing to take it there.
const unitRunCmdConfig = `
locals {
  escaped = run_cmd("--terragrunt-quiet", "sh", "-c", "echo pwned > ${get_terragrunt_dir()}/escaped.txt")
}

inputs = {
  escaped = local.escaped
}
`

// stackConfig is a stack with one unit, and stackConfigBroken is the same file
// with the block left open.
const stackConfig = `
unit "web" {
  source = "./units/web"
  path   = "web"
}
`

const stackConfigBroken = `
unit "web" {
  source = "./units/web"
`

// unitExcludedFrom is a unit an exclude block drops from one action only, so
// asking about the wrong command reports the wrong set of units.
func unitExcludedFrom(action string) string {
	return `
exclude {
  if      = true
  actions = ["` + action + `"]
}
`
}

const unitS3Config = `
remote_state {
  backend = "s3"

  config = {
    bucket = "example-state"
    key    = "s3-backed/tofu.tfstate"
    region = "us-east-1"
  }
}
`

type discoveredComponent struct {
	Path         string   `json:"path"`
	Dependencies []string `json:"dependencies"`
	Excluded     bool     `json:"excluded"`
}

type discoverOutput struct {
	Units     []discoveredComponent `json:"units"`
	Stacks    []discoveredComponent `json:"stacks"`
	Degraded  []string              `json:"degraded"`
	UnitCount int                   `json:"unit_count"`
}

type renderConfigOutput struct {
	ConfigPath string   `json:"config_path"`
	Degraded   []string `json:"degraded"`
	Summary    struct {
		InputKeys    []string `json:"input_keys"`
		Dependencies []string `json:"dependencies"`
	} `json:"summary"`
}

type runOrderUnit struct {
	Path      string   `json:"path"`
	BlockedBy []string `json:"blocked_by"`
}

type runOrderOutput struct {
	Command string         `json:"command"`
	Order   string         `json:"order"`
	Graph   string         `json:"graph"`
	Units   []runOrderUnit `json:"units"`
}

type outputEntry struct {
	Value     any  `json:"value"`
	Sensitive bool `json:"sensitive"`
}

type getOutputsOutput struct {
	Outputs  map[string]outputEntry `json:"outputs"`
	Redacted []string               `json:"redacted"`
	Degraded []string               `json:"degraded"`
}

type validateDiagnostic struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type validateOutput struct {
	Diagnostics []validateDiagnostic `json:"diagnostics"`
	ErrorCount  int                  `json:"error_count"`
	Valid       bool                 `json:"valid"`
}

// newTestTree writes a two-unit tree, where unit "b" depends on unit "a",
// and returns its symlink-resolved root. Resolving matches what the server
// does to its launch dir, so paths compare on both sides.
func newTestTree(t *testing.T) string {
	t.Helper()

	fsys := vfs.NewOSFS()
	dir, err := vfs.EvalSymlinks(fsys, t.TempDir())
	require.NoError(t, err)

	for name, contents := range map[string]string{"a": unitAConfig, "b": unitBConfig} {
		require.NoError(t, fsys.MkdirAll(filepath.Join(dir, name), 0o755))
		require.NoError(
			t,
			vfs.WriteFile(
				fsys,
				filepath.Join(dir, name, "terragrunt.hcl"),
				[]byte(contents),
				0o644,
			),
		)
	}

	return dir
}

// writeTree writes files, keyed by slash-separated path, under a fresh
// symlink-resolved directory and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()

	fsys := vfs.NewOSFS()
	dir, err := vfs.EvalSymlinks(fsys, t.TempDir())
	require.NoError(t, err)

	for name, contents := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, fsys.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(contents), 0o644))
	}

	return dir
}

// newTestSession serves the MCP tool set over a pair of in-memory pipes and
// returns a client session connected to it. Every capability is denied, so
// no tool call can reach a subprocess or the network.
func newTestSession(t *testing.T, dir string) *mcp.ClientSession {
	t.Helper()

	return newTestSessionWithOptions(t, dir, func(*tgmcp.Options) {})
}

// newApprovingSession serves the mutating tools to a client that answers every
// approval prompt with action, and records the prompts it was shown.
func newApprovingSession(t *testing.T, dir, action string, prompts *[]string) *mcp.ClientSession {
	t.Helper()

	return newAnsweringSession(t, dir, venvtest.NewWithOSFS(), action, prompts,
		func(o *tgmcp.Options) {
			o.Allow = []string{"exec"}
			o.AllowApply = true
		})
}

// newAnsweringSession is [newApprovingSession] with the venv and capabilities
// open, for a test whose subject is which programs an approval lets run.
func newAnsweringSession(
	t *testing.T,
	dir string,
	v *venv.Venv,
	action string,
	prompts *[]string,
	configure func(*tgmcp.Options),
) *mcp.ClientSession {
	t.Helper()

	return newTestSessionWith(t, dir, v, configure,
		&mcp.ClientOptions{
			ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				*prompts = append(*prompts, req.Params.Message)

				return &mcp.ElicitResult{Action: action}, nil
			},
		})
}

// newTestSessionWithOptions is [newTestSession] with the command options open
// for a test that needs a capability granted.
func newTestSessionWithOptions(
	t *testing.T,
	dir string,
	configure func(*tgmcp.Options),
) *mcp.ClientSession {
	t.Helper()

	return newTestSessionWith(t, dir, venvtest.NewWithOSFS(), configure, nil)
}

// newRealExecSession is [newTestSessionWithOptions] with a venv that can spawn
// for real, for a test whose subject is which programs get to run.
func newRealExecSession(
	t *testing.T,
	dir string,
	configure func(*tgmcp.Options),
) *mcp.ClientSession {
	t.Helper()

	return newTestSessionWith(t, dir, venvtest.NewOSWithEmptyEnv(), configure, nil)
}

// newTestSessionWith is [newTestSessionWithOptions] with the client options
// open too, for a test that needs the client to answer the server back.
func newTestSessionWith(
	t *testing.T,
	dir string,
	v *venv.Venv,
	configure func(*tgmcp.Options),
	clientOpts *mcp.ClientOptions,
) *mcp.ClientSession {
	t.Helper()

	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(dir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = dir

	cmdOpts := tgmcp.NewOptions(opts)
	configure(cmdOpts)

	ctx, cancel := context.WithCancel(context.Background())

	served := make(chan error, 1)

	go func() {
		served <- tgmcp.Serve(ctx, logger.CreateLogger(), v, cmdOpts, serverIn, serverOut)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "terragrunt-test", Version: "1"}, clientOpts)

	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientIn, Writer: clientOut}, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, session.Close(), "closing the MCP client session")

		cancel()

		select {
		case serveErr := <-served:
			assert.NoError(t, serveErr)
		case <-time.After(30 * time.Second):
			assert.Fail(t, "mcp.Serve did not return after the client disconnected")
		}
	})

	return session
}

// callTool calls the named tool and decodes its structured result into out.
func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any, out any) {
	t.Helper()

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.False(t, res.IsError, "the %s tool call failed: %v", name, res.GetError())

	structured, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(structured, out))
}

func toolNames(t *testing.T, session *mcp.ClientSession) []string {
	t.Helper()

	res, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}

	slices.Sort(names)

	return names
}

func TestServeAdvertisesTheToolSet(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	assert.Equal(t,
		[]string{"discover", "get_outputs", "plan", "render_config", "run_order", "validate"},
		toolNames(t, session))
}

// TestMutatingToolsAreAbsentUntilGranted pins that a client is told apply and
// destroy exist only when the server was started with
// --dangerously-allow-apply.
func TestMutatingToolsAreAbsentUntilGranted(t *testing.T) {
	t.Parallel()

	denied := newTestSession(t, newTestTree(t))
	assert.NotContains(t, toolNames(t, denied), "apply")
	assert.NotContains(t, toolNames(t, denied), "destroy")

	granted := newTestSessionWithOptions(t, newTestTree(t), func(o *tgmcp.Options) {
		o.Allow = []string{"exec"}
		o.AllowApply = true
	})
	assert.Equal(
		t,
		[]string{
			"apply",
			"destroy",
			"discover",
			"get_outputs",
			"plan",
			"render_config",
			"run_order",
			"validate",
		},
		toolNames(t, granted),
	)
}

// TestMutatingToolsAreAnnotatedDestructive pins the annotations an MCP client
// gates on before letting a model call a tool that changes infrastructure.
func TestMutatingToolsAreAnnotatedDestructive(t *testing.T) {
	t.Parallel()

	session := newTestSessionWithOptions(t, newTestTree(t), func(o *tgmcp.Options) {
		o.Allow = []string{"exec"}
		o.AllowApply = true
	})

	res, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)

	seen := 0

	for _, tool := range res.Tools {
		if tool.Name != "apply" && tool.Name != "destroy" {
			continue
		}

		seen++

		assert.False(t, tool.Annotations.ReadOnlyHint, tool.Name)
		assert.False(t, tool.Annotations.IdempotentHint, tool.Name)
		require.NotNil(t, tool.Annotations.DestructiveHint, tool.Name)
		assert.True(t, *tool.Annotations.DestructiveHint, tool.Name)
	}

	assert.Equal(t, 2, seen, "both mutating tools must be advertised")
}

func TestDiscoverToolReportsUnitsAndDependencies(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	var out discoverOutput

	callTool(t, session, "discover", map[string]any{"dependencies": true}, &out)

	assert.Equal(t, 2, out.UnitCount)
	assert.Equal(t, []discoveredComponent{
		{Path: "a"},
		{Path: "b", Dependencies: []string{"a"}},
	}, out.Units)
	assert.Empty(t, out.Degraded)
}

func TestRenderConfigToolSummarizesAUnit(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "b"}, &out)

	assert.Equal(t, filepath.Join("b", "terragrunt.hcl"), out.ConfigPath)
	assert.Equal(t, []string{"a_id"}, out.Summary.InputKeys)
	assert.Equal(t, []string{"a: ../a"}, out.Summary.Dependencies)
}

// TestRunOrderReportsWhatBlocksEachUnit pins that each unit lists the units it
// waits on. Plan and apply wait on dependencies, and destroy waits on
// dependents.
func TestRunOrderReportsWhatBlocksEachUnit(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	for _, tc := range []struct {
		blockedBy map[string][]string
		command   string
		order     string
	}{
		{command: "plan", order: "apply", blockedBy: map[string][]string{"a": nil, "b": {"a"}}},
		{command: "apply", order: "apply", blockedBy: map[string][]string{"a": nil, "b": {"a"}}},
		{command: "destroy", order: "destroy", blockedBy: map[string][]string{"a": {"b"}, "b": nil}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			t.Parallel()

			var out runOrderOutput

			callTool(t, session, "run_order", map[string]any{"command": tc.command}, &out)

			assert.Equal(t, tc.command, out.Command)
			assert.Equal(t, tc.order, out.Order)

			got := map[string][]string{}
			for _, unit := range out.Units {
				got[unit.Path] = unit.BlockedBy
			}

			assert.Equal(t, tc.blockedBy, got)
			assert.Empty(t, out.Graph, "no rendering was asked for")
		})
	}
}

// TestRunOrderRendersTheGraphOnRequest pins the two renderings, which exist so
// a person reading an agent's answer sees the shape the list command already
// shows them.
func TestRunOrderRendersTheGraphOnRequest(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	for _, tc := range []struct {
		format   string
		contains string
	}{
		{format: "tree", contains: "└── a"},
		{format: "dot", contains: "digraph {"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			t.Parallel()

			var out runOrderOutput

			callTool(t, session, "run_order", map[string]any{"format": tc.format}, &out)

			assert.Contains(t, out.Graph, tc.contains)
			assert.Contains(t, out.Graph, "b")
			assert.NotEmpty(t, out.Units, "a rendering does not replace the edges")
		})
	}
}

// TestRunOrderRejectsAnUnknownFormat pins that an unknown format fails the
// call.
func TestRunOrderRejectsAnUnknownFormat(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "run_order",
		Arguments: map[string]any{"format": "graphviz"},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

func TestValidateToolSeparatesCleanAndBrokenTrees(t *testing.T) {
	t.Parallel()

	dir := newTestTree(t)
	session := newTestSession(t, dir)

	var clean validateOutput

	callTool(t, session, "validate", map[string]any{}, &clean)

	assert.True(t, clean.Valid)
	assert.Equal(t, 0, clean.ErrorCount)

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(filepath.Join(dir, "broken"), 0o755))
	require.NoError(
		t,
		vfs.WriteFile(
			fsys,
			filepath.Join(dir, "broken", "terragrunt.hcl"),
			[]byte("inputs = {\n"),
			0o644,
		),
	)

	var broken validateOutput

	callTool(t, session, "validate", map[string]any{"unit": "broken"}, &broken)

	assert.False(t, broken.Valid)
	assert.Positive(t, broken.ErrorCount)
}

func TestToolCallsAreConfinedToTheServerRoot(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "discover",
		Arguments: map[string]any{"working_dir": ".."},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError, "discovery outside the server root must be refused")
}

// TestWorkingDirThroughASymlinkIsConfinedToTheServerRoot pins that the root is
// checked once symlinks resolve. A link inside the tree that points out of it
// reads as a directory under the root until it is followed.
func TestWorkingDirThroughASymlinkIsConfinedToTheServerRoot(t *testing.T) {
	t.Parallel()

	dir := newTestTree(t)
	outside := writeTree(t, map[string]string{"u/terragrunt.hcl": "\n"})
	require.NoError(t, vfs.Symlink(vfs.NewOSFS(), outside, filepath.Join(dir, "link")))

	session := newTestSession(t, dir)

	for _, tc := range []struct {
		tool       string
		workingDir string
	}{
		{tool: "discover", workingDir: "link"},
		{tool: "render_config", workingDir: "link/u"},
	} {
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      tc.tool,
			Arguments: map[string]any{"working_dir": tc.workingDir},
		})
		require.NoError(t, err)
		assert.True(t, res.IsError, "%s on %q must be refused", tc.tool, tc.workingDir)
	}
}

func TestPlanIsRefusedWithoutAllowExec(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "plan",
		Arguments: map[string]any{},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError, "plan must be refused when the server cannot spawn subprocesses")
}

// TestConcurrentToolCallsShareOneServerWithRacing drives every read-only tool
// at once over a single session. The SDK dispatches each call on its own
// goroutine, so the per-call option builders, caches, and exec recorders must
// not be shared between them.
func TestConcurrentToolCallsShareOneServerWithRacing(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	calls := []struct {
		args map[string]any
		name string
	}{
		{name: "discover", args: map[string]any{"dependencies": true}},
		{name: "render_config", args: map[string]any{"working_dir": "a"}},
		{name: "render_config", args: map[string]any{"working_dir": "b"}},
		{name: "run_order", args: map[string]any{}},
		{name: "validate", args: map[string]any{}},
	}

	var g errgroup.Group

	for _, call := range calls {
		g.Go(func() error {
			res, err := session.CallTool(
				t.Context(),
				&mcp.CallToolParams{Name: call.name, Arguments: call.args},
			)
			if err != nil {
				return fmt.Errorf("calling the %s tool: %w", call.name, err)
			}

			assert.False(
				t,
				res.IsError,
				"the %s tool call failed: %v",
				call.name,
				res.GetError(),
			)

			return nil
		})
	}

	require.NoError(t, g.Wait())
}

// TestToolAnnotationsFollowTheGrantedCapabilities pins the open-world hint a
// client reads to decide whether a tool can leave the machine. It is set only
// when the server may spawn a process or make a request.
func TestToolAnnotationsFollowTheGrantedCapabilities(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		configure func(*tgmcp.Options)
		name      string
		openWorld bool
	}{
		{
			name:      "no capabilities granted",
			configure: func(*tgmcp.Options) {},
			openWorld: false,
		},
		{
			name:      "http granted",
			configure: func(o *tgmcp.Options) { o.Allow = []string{"http"} },
			openWorld: true,
		},
		{
			name:      "exec granted",
			configure: func(o *tgmcp.Options) { o.Allow = []string{"exec"} },
			openWorld: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			session := newTestSessionWithOptions(t, newTestTree(t), tc.configure)

			res, err := session.ListTools(t.Context(), nil)
			require.NoError(t, err)

			for _, tool := range res.Tools {
				// plan is the exception: it requires --allow=exec, and the
				// command it spawns reaches the network on its own account.
				if tool.Name == "plan" {
					continue
				}

				require.NotNil(t, tool.Annotations.OpenWorldHint, tool.Name)
				assert.Equal(t, tc.openWorld, *tool.Annotations.OpenWorldHint, tool.Name)
			}
		})
	}
}

// s3StateObject is the state the unitS3Config backend points at, with one
// output.
const s3StateObject = `{"version":4,"serial":1,"lineage":"test","outputs":{"id":{"value":"from-state","type":"string"}},"resources":[]}`

// TestGetOutputsReadsRemoteStateOnlyUnderAllowHTTP pins the http capability
// on a request Terragrunt makes itself. Without exec, get_outputs reads an s3
// backend's state object directly. Denied, it sends no request and says why it
// returned nothing. Granted, it returns the outputs in the object.
func TestGetOutputsReadsRemoteStateOnlyUnderAllowHTTP(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		requested assert.BoolAssertionFunc
		want      map[string]outputEntry
		name      string
		allow     []string
	}{
		{
			name:      "denied",
			allow:     []string{"env"},
			requested: assert.False,
			want:      map[string]outputEntry{},
		},
		{
			name:      "granted",
			allow:     []string{"env", "http"},
			requested: assert.True,
			want:      map[string]outputEntry{"id": {Value: "from-state"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := writeTree(t, map[string]string{"s3-backed/terragrunt.hcl": unitS3Config})

			var requests atomic.Int32

			s3 := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
				requests.Add(1)

				if !strings.HasSuffix(req.URL.Path, "/s3-backed/tofu.tfstate") {
					return vhttp.Respond(http.StatusNotFound, nil, nil), nil
				}

				return vhttp.Respond(http.StatusOK, []byte(s3StateObject), nil), nil
			})

			// Static keys stop the SDK from resolving credentials on the
			// machine running the test.
			v := venvtest.NewWithOSFS().WithHTTP(s3).WithEnv(map[string]string{
				"AWS_ACCESS_KEY_ID":     "AKIATEST",
				"AWS_SECRET_ACCESS_KEY": "test-secret",
			})

			session := newTestSessionWith(t, dir, v, func(o *tgmcp.Options) { o.Allow = tc.allow }, nil)

			var out getOutputsOutput

			callTool(t, session, "get_outputs", map[string]any{"working_dir": "s3-backed"}, &out)

			assert.Equal(t, tc.want, out.Outputs)
			assert.NotEmpty(t, out.Degraded)
			tc.requested(t, requests.Load() > 0, "whether the state object was requested")
		})
	}
}

// TestDestroyAsksBeforeActing pins that destroy asks once, naming the units it
// would touch, and reports a declined run as a failure.
func TestDestroyAsksBeforeActing(t *testing.T) {
	t.Parallel()

	var prompts []string

	session := newApprovingSession(t, newTestTree(t), "decline", &prompts)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "destroy",
		Arguments: map[string]any{},
	})
	require.NoError(t, err)

	require.Len(t, prompts, 1, "the call must ask exactly once")
	assert.Contains(t, prompts[0], "b")
	assert.Contains(t, prompts[0], "a")

	assert.True(t, res.IsError, "a declined run must not report success")
}

// TestDestroyDeclinedRunsNothing pins that a refusal is final: the units are
// still there afterwards, and asking again asks again.
func TestDestroyDeclinedRunsNothing(t *testing.T) {
	t.Parallel()

	dir := newTestTree(t)

	var prompts []string

	session := newApprovingSession(t, dir, "cancel", &prompts)

	for range 2 {
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      "destroy",
			Arguments: map[string]any{},
		})
		require.NoError(t, err)
		assert.True(t, res.IsError)
	}

	assert.Len(t, prompts, 2, "each call must ask on its own account")

	for _, unit := range []string{"a", "b"} {
		exists, err := vfs.FileExists(vfs.NewOSFS(), filepath.Join(dir, unit, "terragrunt.hcl"))
		require.NoError(t, err)
		assert.True(t, exists, "declining must leave the tree alone")
	}
}

// TestAllowExecDoesNotRunProgramsFromConfiguration pins the boundary
// --allow=exec draws. It grants the commands Terragrunt runs itself, and a
// program named by a configuration under the server root stays refused,
// because that is the path from a repository an agent was pointed at to code
// running on this machine.
func TestAllowExecDoesNotRunProgramsFromConfiguration(t *testing.T) {
	t.Parallel()

	dir := newTestTree(t)
	unit := filepath.Join(dir, "shell")

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(unit, 0o755))
	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(unit, "terragrunt.hcl"), []byte(unitRunCmdConfig), 0o644),
	)

	// The client cannot elicit, so the call reports the refused program in its
	// degraded list.
	session := newRealExecSession(
		t,
		dir,
		func(o *tgmcp.Options) { o.Allow = []string{"exec"} },
	)

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "shell"}, &out)

	assert.NotEmpty(t, out.Degraded, "a refused program must be reported, not silently dropped")

	exists, err := vfs.FileExists(fsys, filepath.Join(unit, "escaped.txt"))
	require.NoError(t, err)
	assert.False(t, exists, "the program a configuration named must not have run")
}

// newShellUnitTree writes a tree holding one unit whose local calls a program,
// and returns the tree root and the file that program would create.
func newShellUnitTree(t *testing.T) (string, string) {
	t.Helper()

	dir := newTestTree(t)
	unit := filepath.Join(dir, "shell")
	marker := filepath.Join(unit, "escaped.txt")

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(unit, 0o755))
	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(unit, "terragrunt.hcl"), []byte(unitRunCmdConfig), 0o644),
	)

	return dir, marker
}

// TestCommandApprovalOffersRefusedProgramsOnce pins that a call meeting a
// refused program asks once, naming the program, and that a declined offer
// leaves the program unrun and reported in the degraded list.
func TestCommandApprovalOffersRefusedProgramsOnce(t *testing.T) {
	t.Parallel()

	dir, marker := newShellUnitTree(t)

	var prompts []string

	session := newAnsweringSession(t, dir, venvtest.NewOSWithEmptyEnv(), "decline", &prompts,
		func(o *tgmcp.Options) { o.Allow = []string{"exec"} })

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "shell"}, &out)

	require.Len(t, prompts, 1, "the call must offer the programs exactly once")
	assert.Contains(t, prompts[0], "sh")

	assert.NotEmpty(t, out.Degraded, "declining still reports what was substituted")

	exists, err := vfs.FileExists(vfs.NewOSFS(), marker)
	require.NoError(t, err)
	assert.False(t, exists, "declining must leave the program unrun")
}

// TestCommandApprovalRunsWhatWasApproved pins the other half: accepting the
// offer runs the program, which is the whole point of asking.
func TestCommandApprovalRunsWhatWasApproved(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping on Windows: the unit runs sh, which Windows does not ship")
	}

	dir, marker := newShellUnitTree(t)

	var prompts []string

	session := newAnsweringSession(t, dir, venvtest.NewOSWithEmptyEnv(), "accept", &prompts,
		func(o *tgmcp.Options) { o.Allow = []string{"exec"} })

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "shell"}, &out)

	require.Len(t, prompts, 1, "an accepted offer must not be made again")

	exists, err := vfs.FileExists(vfs.NewOSFS(), marker)
	require.NoError(t, err)
	assert.True(t, exists, "the approved program must have run")
}

// TestAllowCmdRunsWithoutAsking pins that a command an --allow-cmd pattern
// matches runs without a prompt.
func TestAllowCmdRunsWithoutAsking(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping on Windows: the unit runs sh, which Windows does not ship")
	}

	dir, marker := newShellUnitTree(t)

	var prompts []string

	session := newAnsweringSession(t, dir, venvtest.NewOSWithEmptyEnv(), "decline", &prompts,
		func(o *tgmcp.Options) {
			o.Allow = []string{"exec"}
			o.AllowCmds = []string{"sh -c *"}
		})

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "shell"}, &out)

	assert.Empty(t, prompts, "an allowed command must not be offered for approval")

	exists, err := vfs.FileExists(vfs.NewOSFS(), marker)
	require.NoError(t, err)
	assert.True(t, exists, "the allowed command must have run")
}

// TestAllowCmdRefusesArgumentsItDoesNotMatch pins that a pattern allows the
// arguments it names and no others. The program is the same, so a name-only
// allowlist would have run it.
func TestAllowCmdRefusesArgumentsItDoesNotMatch(t *testing.T) {
	t.Parallel()

	dir, marker := newShellUnitTree(t)

	var prompts []string

	session := newAnsweringSession(t, dir, venvtest.NewOSWithEmptyEnv(), "decline", &prompts,
		func(o *tgmcp.Options) {
			o.Allow = []string{"exec"}
			o.AllowCmds = []string{`sh -c "echo hello"`}
		})

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "shell"}, &out)

	assert.NotEmpty(t, out.Degraded, "the refused command must be reported")

	exists, err := vfs.FileExists(vfs.NewOSFS(), marker)
	require.NoError(t, err)
	assert.False(t, exists, "a command whose arguments the pattern does not match must not run")
}

// TestStacksAreDiscoveredAndValidated covers the terragrunt.stack.hcl path,
// which discover reports separately from units and validate parses with the
// strict autoinclude check that stack generate uses.
func TestStacksAreDiscoveredAndValidated(t *testing.T) {
	t.Parallel()

	dir := newTestTree(t)
	stackDir := filepath.Join(dir, "estate")

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(stackDir, 0o755))
	require.NoError(t, vfs.WriteFile(
		fsys, filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(stackConfig), 0o644))

	session := newTestSession(t, dir)

	var discovered discoverOutput

	callTool(t, session, "discover", map[string]any{}, &discovered)

	assert.Equal(t, 2, discovered.UnitCount, "a stack is not a unit")
	assert.Equal(t, []discoveredComponent{{Path: "estate"}}, discovered.Stacks)

	var valid validateOutput

	callTool(t, session, "validate", map[string]any{"unit": "estate"}, &valid)

	assert.True(t, valid.Valid, "a well-formed stack must validate")
}

// TestValidateReportsABrokenStackFile pins that a broken stack file gets a
// diagnostic naming its file and line.
func TestValidateReportsABrokenStackFile(t *testing.T) {
	t.Parallel()

	dir := newTestTree(t)
	stackDir := filepath.Join(dir, "estate")

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(stackDir, 0o755))
	require.NoError(t, vfs.WriteFile(
		fsys, filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(stackConfigBroken), 0o644))

	session := newTestSession(t, dir)

	var out validateOutput

	callTool(t, session, "validate", map[string]any{"unit": "estate"}, &out)

	assert.False(t, out.Valid)
	assert.Positive(t, out.ErrorCount)
	require.NotEmpty(t, out.Diagnostics)
	assert.Equal(t, filepath.Join("estate", "terragrunt.stack.hcl"), out.Diagnostics[0].File)
	assert.Positive(t, out.Diagnostics[0].Line)
}

// TestRunOrderHonoursTheCommandsExcludeBlocks pins that the answer is about
// the command that was asked for. An exclude block naming one action does not
// match another, so asking about plan and asking about apply cover different
// units, and answering both with the same set would send an agent to run
// something a real apply would have skipped.
func TestRunOrderHonoursTheCommandsExcludeBlocks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	fsys := vfs.NewOSFS()

	for action, unit := range map[string]string{"plan": "skips-plan", "apply": "skips-apply"} {
		require.NoError(t, fsys.MkdirAll(filepath.Join(dir, unit), 0o755))
		require.NoError(t, vfs.WriteFile(
			fsys,
			filepath.Join(dir, unit, "terragrunt.hcl"),
			[]byte(unitExcludedFrom(action)),
			0o644,
		))
	}

	session := newTestSession(t, dir)

	for command, want := range map[string][]string{
		"plan":  {"skips-apply"},
		"apply": {"skips-plan"},
	} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			var out runOrderOutput

			callTool(t, session, "run_order", map[string]any{"command": command}, &out)

			paths := make([]string, 0, len(out.Units))
			for _, unit := range out.Units {
				paths = append(paths, unit.Path)
			}

			assert.Equal(
				t,
				want,
				paths,
				"the unit excluded from %s must be the one missing",
				command,
			)
		})
	}
}

// TestRunOrderRejectsACommandItCannotOrder pins that run_order refuses a
// command other than plan, apply, or destroy. Exclude blocks name the action
// they apply to, so ordering another command as a plan would report the wrong
// units.
func TestRunOrderRejectsACommandItCannotOrder(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "run_order",
		Arguments: map[string]any{"command": "refresh"},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// TestDiscoverHonoursTheCommandExcludeBlocksAreEvaluatedAgainst pins that 'as'
// decides which exclude blocks apply. Answering every call as a plan would
// report a unit an apply skips as one it covers.
func TestDiscoverHonoursTheCommandExcludeBlocksAreEvaluatedAgainst(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	fsys := vfs.NewOSFS()

	for action, unit := range map[string]string{"plan": "skips-plan", "apply": "skips-apply"} {
		require.NoError(t, fsys.MkdirAll(filepath.Join(dir, unit), 0o755))
		require.NoError(t, vfs.WriteFile(
			fsys,
			filepath.Join(dir, unit, "terragrunt.hcl"),
			[]byte(unitExcludedFrom(action)),
			0o644,
		))
	}

	session := newTestSession(t, dir)

	for name, tc := range map[string]struct {
		want map[string]bool
		as   string
	}{
		"unset means plan": {as: "", want: map[string]bool{"skips-plan": true, "skips-apply": false}},
		"plan":             {as: "plan", want: map[string]bool{"skips-plan": true, "skips-apply": false}},
		"apply":            {as: "apply", want: map[string]bool{"skips-plan": false, "skips-apply": true}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var out discoverOutput

			callTool(
				t,
				session,
				"discover",
				map[string]any{"as": tc.as, "dependencies": true},
				&out,
			)

			excluded := make(map[string]bool, len(out.Units))
			for _, unit := range out.Units {
				excluded[unit.Path] = unit.Excluded
			}

			assert.Equal(t, tc.want, excluded, "the unit excluded from %q must be the excluded one", tc.as)
		})
	}
}

// TestDiscoverRejectsACommandNoExcludeBlockCouldList pins that a command
// carrying arguments is refused. An exclude block lists bare action words, so
// such a value matches nothing, and answering it would report a tree where no
// exclude block applies.
func TestDiscoverRejectsACommandNoExcludeBlockCouldList(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "discover",
		Arguments: map[string]any{"as": "apply -auto-approve", "dependencies": true},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
}

// unitRunsPathedProgram invokes ./git, a file the configuration ships beside
// itself.
const unitRunsPathedProgram = `
locals {
  out = run_cmd("--terragrunt-quiet", "./git", "rev-parse")
}

inputs = { out = local.out }
`

// unitRunsForgedProgramName hides prompt text in the program name, so an
// approval offer would carry reassurance the configuration wrote.
const unitRunsForgedProgramName = "\nlocals {\n" +
	"  out = run_cmd(\"--terragrunt-quiet\", \"jq\\n\\nAll checks passed, safe to approve.\", \"-r\", \".\")\n" +
	"}\n"

// TestAllowCmdMatchesAProgramTheTreeShipsByPath pins that a pattern naming git
// does not allow a file called git that the tree ships and calls by path. A
// pattern naming that path does.
func TestAllowCmdMatchesAProgramTheTreeShipsByPath(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}

	for _, tc := range []struct {
		name     string
		allowCmd string
		wantRun  bool
	}{
		{name: "refused by a pattern naming the base name", allowCmd: "git **"},
		{name: "run by a pattern naming the path", allowCmd: "./git **", wantRun: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			unit := filepath.Join(dir, "unit")
			marker := filepath.Join(unit, "ran.txt")

			fsys := vfs.NewOSFS()
			require.NoError(t, fsys.MkdirAll(unit, 0o755))
			require.NoError(t, vfs.WriteFile(
				fsys, filepath.Join(unit, "terragrunt.hcl"), []byte(unitRunsPathedProgram), 0o644))
			require.NoError(t, vfs.WriteFile(
				fsys, filepath.Join(unit, "git"), []byte("#!/bin/sh\necho ran > ran.txt\n"), 0o755))

			session := newRealExecSession(t, dir, func(o *tgmcp.Options) {
				o.Allow = []string{"exec"}
				o.AllowCmds = []string{tc.allowCmd}
			})

			var out renderConfigOutput

			callTool(t, session, "render_config", map[string]any{"working_dir": "unit"}, &out)

			exists, err := vfs.FileExists(fsys, marker)
			require.NoError(t, err)
			assert.Equal(t, tc.wantRun, exists)
		})
	}
}

// TestApprovalPromptDoesNotCarryForgedText pins that a program name cannot
// write into the prompt a person is asked to approve. The name comes from the
// tree the agent chose, so a newline in it would let that tree add its own
// reassurance to a security question.
func TestApprovalPromptDoesNotCarryForgedText(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	unit := filepath.Join(dir, "unit")

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(unit, 0o755))
	require.NoError(t, vfs.WriteFile(
		fsys, filepath.Join(unit, "terragrunt.hcl"), []byte(unitRunsForgedProgramName), 0o644))

	var prompts []string

	session := newAnsweringSession(t, dir, venvtest.NewOSWithEmptyEnv(), "decline", &prompts,
		func(o *tgmcp.Options) { o.Allow = []string{"exec"} })

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "unit"}, &out)

	require.Len(t, prompts, 1)

	// A short enough string will always fit inside a name, so the guarantee
	// worth pinning is structural: one refused program is one line, and
	// nothing the tree wrote can open a line of its own and read as the
	// server's own words.
	bullets := 0

	for line := range strings.SplitSeq(prompts[0], "\n") {
		if strings.HasPrefix(line, "  - ") {
			bullets++
		}
	}

	assert.Equal(t, 1, bullets, "one refused program must produce one line")
	assert.NotContains(t, prompts[0], "\n\nAll checks passed")
}

// unitRunsGit names git in a local, so evaluating the unit runs git only if
// the allowlist lets a configuration-named program through.
const unitRunsGit = `
locals {
  git = run_cmd("--terragrunt-quiet", "git", "--version")
}

inputs = {
  git = local.git
}
`

// TestConfigurationCannotRunTerragruntsOwnPrograms pins that --allow=exec
// allows git when Terragrunt starts it, and refuses a git a configuration
// names for itself until an --allow-cmd pattern matches it.
func TestConfigurationCannotRunTerragruntsOwnPrograms(t *testing.T) {
	t.Parallel()

	newSession := func(t *testing.T, configure func(*tgmcp.Options)) *mcp.ClientSession {
		t.Helper()

		dir := newTestTree(t)
		unit := filepath.Join(dir, "unit")

		fsys := vfs.NewOSFS()
		require.NoError(t, fsys.MkdirAll(unit, 0o755))
		require.NoError(t, vfs.WriteFile(
			fsys, filepath.Join(unit, "terragrunt.hcl"), []byte(unitRunsGit), 0o644))

		return newRealExecSession(t, dir, configure)
	}

	readGit := func(t *testing.T, session *mcp.ClientSession) string {
		t.Helper()

		var out struct {
			Config struct {
				Inputs map[string]any `json:"inputs"`
			} `json:"config"`
		}

		callTool(t, session, "render_config", map[string]any{"working_dir": "unit", "format": "full"}, &out)

		git, _ := out.Config.Inputs["git"].(string)

		return git
	}

	t.Run("refused with --allow=exec alone", func(t *testing.T) {
		t.Parallel()

		session := newSession(t, func(o *tgmcp.Options) { o.Allow = []string{"exec"} })

		assert.Empty(t, readGit(t, session), "a git the configuration named must not run")
	})

	t.Run("runs when an --allow-cmd pattern matches", func(t *testing.T) {
		t.Parallel()

		session := newSession(t, func(o *tgmcp.Options) {
			o.Allow = []string{"exec"}
			o.AllowCmds = []string{"git --version"}
		})

		assert.Contains(t, readGit(t, session), "git version",
			"an --allow-cmd pattern must let the named git run")
	})
}

// TestEachCallSeesTheTreeAsItIsNow pins that discovery is redone per call. An
// agent edits a unit and asks again, so a result held across calls would keep
// answering about the tree as it was when the client connected.
func TestEachCallSeesTheTreeAsItIsNow(t *testing.T) {
	t.Parallel()

	dir := newTestTree(t)
	session := newTestSession(t, dir)

	var before discoverOutput

	callTool(t, session, "discover", map[string]any{}, &before)
	require.Equal(t, 2, before.UnitCount)

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(filepath.Join(dir, "c"), 0o755))
	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(dir, "c", "terragrunt.hcl"), []byte("\n"), 0o644),
	)

	var after discoverOutput

	callTool(t, session, "discover", map[string]any{}, &after)
	assert.Equal(t, 3, after.UnitCount, "a unit added after the session started must be visible")
}

// unitDecryptsASecret takes its one input from sops_decrypt_file.
const unitDecryptsASecret = `
locals {
  secret = sops_decrypt_file("secrets.json")
}

inputs = { secret = local.secret }
`

// sopsCleartext is the cleartext the decrypter in [newSopsSession] returns for
// every file.
const sopsCleartext = "decrypted-secret"

// newSopsSession serves a tree whose unit u decrypts a file, through a
// decrypter that answers every file with [sopsCleartext]. It returns the
// session and the number of decrypts the decrypter performed.
func newSopsSession(t *testing.T, allow []string) (*mcp.ClientSession, *atomic.Int32) {
	t.Helper()

	dir := writeTree(t, map[string]string{
		"u/terragrunt.hcl": unitDecryptsASecret,
		"u/secrets.json":   `{"sops":"encrypted"}`,
	})

	decrypts := &atomic.Int32{}

	v := venvtest.NewWithOSFS().WithSops(vsops.NewMemDecrypter(
		func(map[string]string, string, string) ([]byte, error) {
			decrypts.Add(1)

			return []byte(sopsCleartext), nil
		},
	))

	return newTestSessionWith(t, dir, v, func(o *tgmcp.Options) { o.Allow = allow }, nil), decrypts
}

// TestSopsDecryptIsRefusedWithoutAllowSops pins that a denied decrypt fails
// the render and never reaches the decrypter.
func TestSopsDecryptIsRefusedWithoutAllowSops(t *testing.T) {
	t.Parallel()

	session, decrypts := newSopsSession(t, nil)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "render_config",
		Arguments: map[string]any{"working_dir": "u"},
	})
	require.NoError(t, err)

	assert.True(t, res.IsError)
	assert.Zero(t, decrypts.Load())
}

// TestSopsDecryptUnderAllowSopsReachesTheResult pins that a granted decrypt
// hands the cleartext to the configuration, and from there to the result.
func TestSopsDecryptUnderAllowSopsReachesTheResult(t *testing.T) {
	t.Parallel()

	session, decrypts := newSopsSession(t, []string{"sops"})

	var out struct {
		Config struct {
			Inputs map[string]any `json:"inputs"`
		} `json:"config"`
	}

	callTool(t, session, "render_config", map[string]any{"working_dir": "u", "format": "full"}, &out)

	assert.Equal(t, sopsCleartext, out.Config.Inputs["secret"])
	assert.Positive(t, decrypts.Load())
}

// TestGraphTraversalStaysInsideTheServerRoot pins the discovery boundary the
// server sets on every call. A filter that follows the graph walks out to the
// git repository root by default, which would hand an agent units from a tree
// it was never pointed at.
func TestGraphTraversalStaysInsideTheServerRoot(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside", "b")

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(filepath.Join(root, "a"), 0o755))
	require.NoError(t, fsys.MkdirAll(outside, 0o755))
	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(outside, "terragrunt.hcl"), []byte("\n"), 0o644),
	)
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "a", "terragrunt.hcl"), []byte(`
dependency "out" {
  config_path  = "../../outside/b"
  mock_outputs = { id = "m" }
}

inputs = { id = dependency.out.outputs.id }
`), 0o644))

	session := newTestSession(t, root)

	var out discoverOutput

	callTool(t, session, "discover", map[string]any{"filter": []string{"{./a}..."}}, &out)

	paths := make([]string, 0, len(out.Units))
	for _, unit := range out.Units {
		paths = append(paths, unit.Path)
	}

	assert.Equal(t, []string{"a"}, paths, "a unit outside the server root must not be returned")
}
