package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
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

type getOutputsOutput struct {
	Outputs  map[string]any `json:"outputs"`
	Degraded []string       `json:"degraded"`
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
		if closeErr := session.Close(); closeErr != nil {
			t.Errorf("closing the MCP client session: %v", closeErr)
		}

		cancel()

		select {
		case serveErr := <-served:
			assert.NoError(t, serveErr)
		case <-time.After(30 * time.Second):
			t.Fatal("mcp.Serve did not return after the client disconnected")
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

// TestMutatingToolsAreAbsentUntilGranted pins that a client without the
// mutation capability is never told an apply or destroy tool exists, rather
// than being offered tools that would refuse.
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

// TestRunOrderReportsWhatBlocksEachUnit pins the contract that replaced the old
// wave grouping. Terragrunt starts a unit as soon as the units blocking it have
// finished, so the answer is per-unit edges. Plan and apply block on
// dependencies, destroy on dependents.
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

// TestRunOrderRejectsAnUnknownFormat pins that a typo is refused rather than
// silently falling back to the default.
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

func TestExecDependentToolsAreRefusedWithoutAllowExec(t *testing.T) {
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

// TestToolAnnotationsFollowTheGrantedCapabilities pins what the server tells a
// client about itself. The open-world hint is how an MCP client decides
// whether a tool can leave the machine, so it has to follow the capabilities
// this server was granted rather than being fixed at registration.
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

// TestGetOutputsDegradesWithoutTheNetwork pins the contract for a unit whose
// outputs are only reachable over the wire: the call succeeds and says what it
// could not do, rather than failing, so an agent can act on the explanation.
func TestGetOutputsDegradesWithoutTheNetwork(t *testing.T) {
	t.Parallel()

	dir := newTestTree(t)

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(filepath.Join(dir, "s3-backed"), 0o755))
	require.NoError(t, vfs.WriteFile(
		fsys, filepath.Join(dir, "s3-backed", "terragrunt.hcl"), []byte(unitS3Config), 0o644))

	session := newTestSession(t, dir)

	var out getOutputsOutput

	callTool(t, session, "get_outputs", map[string]any{"working_dir": "s3-backed"}, &out)

	assert.Empty(t, out.Outputs)
	assert.NotEmpty(t, out.Degraded, "a call that fetched nothing must say why")
}

// TestServerSpeaksTheMultiRoundTripProtocol pins the protocol revision the
// server negotiates with a current client. The approval flow the mutating
// tools depend on is carried by input requests embedded in a result, which
// only exists from this revision on; an older negotiated version would mean
// the SDK was quietly fulfilling them behind our back instead.
func TestServerSpeaksTheMultiRoundTripProtocol(t *testing.T) {
	t.Parallel()

	session := newTestSession(t, newTestTree(t))

	res := session.InitializeResult()
	require.NotNil(t, res)
	assert.Equal(t, "2026-07-28", res.ProtocolVersion)
}

// TestDestroyAsksBeforeActing pins the gate on the path that needs no
// subprocess: the call puts the exact set of units in front of a person, and
// naming them is what makes the approval mean something.
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

	// The client cannot elicit, so the call degrades rather than offering the
	// program for approval.
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

// TestCommandApprovalOffersRefusedProgramsOnce pins the two-phase offer. The
// first call answers with the programs the configuration wanted rather than a
// result built without them, and names them, so what is approved is a specific
// program rather than the idea of one.
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

// TestAllowCommandRunsWithoutAsking pins the flag: a program named up front
// runs with no prompt at all, which is what an operator wants once they know
// which programs a tree needs.
func TestAllowCommandRunsWithoutAsking(t *testing.T) {
	t.Parallel()

	dir, marker := newShellUnitTree(t)

	var prompts []string

	session := newAnsweringSession(t, dir, venvtest.NewOSWithEmptyEnv(), "decline", &prompts,
		func(o *tgmcp.Options) {
			o.Allow = []string{"exec"}
			o.AllowCommands = []string{"sh"}
		})

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "shell"}, &out)

	assert.Empty(t, prompts, "an allowed program must not be offered for approval")

	exists, err := vfs.FileExists(vfs.NewOSFS(), marker)
	require.NoError(t, err)
	assert.True(t, exists, "the allowed program must have run")
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

// TestValidateReportsABrokenStackFile pins that a stack file gets the same
// file-and-line diagnostic a broken unit does, rather than an opaque failure.
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

// TestRunOrderRejectsACommandItCannotOrder pins that a command outside the
// three it understands is refused rather than silently answered as a plan,
// which would report an exclude set belonging to a different action.
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

// unitRunsPathedProgram invokes a program by a relative path, so the file the
// configuration ships beside itself is what would run.
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

// TestAllowExecRefusesAProgramTheTreeShips pins the hole a base-name allowlist
// leaves. A configuration that ships a file named after an allowed program and
// calls it by path would otherwise reach the real executor, which is the exact
// thing --allow=exec is meant not to grant.
func TestAllowExecRefusesAProgramTheTreeShips(t *testing.T) {
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
		o.AllowCommands = []string{"git"}
	})

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "unit"}, &out)

	exists, err := vfs.FileExists(fsys, marker)
	require.NoError(t, err)
	assert.False(t, exists, "a program named by path must not run, whatever its base name")
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

// unitNamesItsOwnBinary picks the tofu binary from configuration, which
// Terragrunt honours whenever the operator left --tf-path alone.
const unitNamesItsOwnBinary = `
terraform_binary = "./tofu"
`

// TestConfigurationCannotChooseTheBinary pins that terraform_binary does not
// reach the exec allowlist. A tree under the server root naming its own binary
// would otherwise pick the program the allowlist trusts, which is the same
// escape a pathed run_cmd would be.
func TestConfigurationCannotChooseTheBinary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	unit := filepath.Join(dir, "unit")
	marker := filepath.Join(unit, "ran.txt")

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(unit, 0o755))
	require.NoError(t, vfs.WriteFile(
		fsys, filepath.Join(unit, "terragrunt.hcl"), []byte(unitNamesItsOwnBinary), 0o644))
	require.NoError(t, vfs.WriteFile(
		fsys, filepath.Join(unit, "tofu"), []byte("#!/bin/sh\necho ran > ran.txt\n"), 0o755))

	session := newRealExecSession(
		t,
		dir,
		func(o *tgmcp.Options) { o.Allow = []string{"exec"} },
	)

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "unit"}, &out)

	exists, err := vfs.FileExists(fsys, marker)
	require.NoError(t, err)
	assert.False(t, exists, "a binary the configuration named must not run")
}

// TestOperatorTFPathIsHonoured pins the other half: a binary the operator named
// with --tf-path is the one the tools run, path and all.
func TestOperatorTFPathIsHonoured(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tfPath := filepath.Join(t.TempDir(), "my-tofu")

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(filepath.Join(dir, "unit"), 0o755))
	require.NoError(t, vfs.WriteFile(
		fsys, filepath.Join(dir, "unit", "terragrunt.hcl"), []byte("\n"), 0o644))

	session := newTestSessionWithOptions(t, dir, func(o *tgmcp.Options) {
		o.Allow = []string{"exec"}
		o.TFPath = tfPath
		o.TFPathExplicitlySet = true
	})

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "unit"}, &out)

	assert.Empty(t, out.Degraded, "the operator's binary must not be reported as refused")
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

// unitDecryptsASecret reads a value through sops_decrypt_file, which fails
// until the sops capability is granted.
const unitDecryptsASecret = `
locals {
  secret = sops_decrypt_file("secrets.json")
}

inputs = { s = local.secret }
`

// TestSopsDenialNamesTheCapabilityItNeeds pins that guidance points at the
// capability actually denied. An HCL function failure is rebuilt as a
// diagnostic and loses the Go error it wrapped, so a message that prescribed a
// capability of its own would name the wrong one and send an operator to grant
// execution for a decrypt.
func TestSopsDenialNamesTheCapabilityItNeeds(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	unit := filepath.Join(dir, "u")

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(unit, 0o755))
	require.NoError(t, vfs.WriteFile(
		fsys, filepath.Join(unit, "terragrunt.hcl"), []byte(unitDecryptsASecret), 0o644))
	require.NoError(t, vfs.WriteFile(
		fsys, filepath.Join(unit, "secrets.json"), []byte(`{"a":"b"}`), 0o644))

	session := newTestSession(t, dir)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "render_config",
		Arguments: map[string]any{"working_dir": "u"},
	})
	require.NoError(t, err)
	require.True(t, res.IsError, "a refused decrypt fails the render")

	require.NotEmpty(t, res.Content)

	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok, "a failed call reports its reason as text")

	message := text.Text
	assert.Contains(t, message, "--allow=sops")
	assert.NotContains(t, message, "Restart the MCP server with --allow=exec",
		"a decrypt is not fixed by granting execution")
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
