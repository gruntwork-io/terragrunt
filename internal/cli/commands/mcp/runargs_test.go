package mcp_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOperatorTFPathPicksTheImplementation pins that --tf-path names the
// binary a tool call starts, as a program on the PATH or as a path. Exec is
// denied, so the degraded list names the refused command, and a binary under
// any file name is refused the way tofu is.
func TestOperatorTFPathPicksTheImplementation(t *testing.T) {
	t.Parallel()

	for _, tfPath := range []string{"terraform", "/abs/path/my-tofu"} {
		t.Run(tfPath, func(t *testing.T) {
			t.Parallel()

			dir := newTestTree(t)

			require.NoError(t, vfs.WriteFile(
				vfs.NewOSFS(), filepath.Join(dir, "a", "main.tf"), []byte(`output "id" { value = "a" }`+"\n"), 0o644))

			session := newTestSessionWithOptions(t, dir, func(o *tgmcp.Options) {
				o.TFPath = tfPath
				o.TFPathExplicitlySet = true
			})

			var out renderConfigOutput

			callTool(t, session, "render_config", map[string]any{"working_dir": "b"}, &out)

			denied := `denied "` + tfPath + " "
			assert.True(t, slices.ContainsFunc(out.Degraded, func(note string) bool {
				return strings.HasPrefix(note, denied)
			}), "the dependency fetch must be refused as %s, got %v", tfPath, out.Degraded)
		})
	}
}

// TestRunArgumentsAreCheckedBeforeAnyoneIsAsked pins that a run given an
// argument it cannot honor fails outright, and that apply fails before it puts
// an approval in front of a person for a run that would then be refused.
func TestRunArgumentsAreCheckedBeforeAnyoneIsAsked(t *testing.T) {
	t.Parallel()

	for _, tool := range []string{"plan", "apply"} {
		t.Run(tool, func(t *testing.T) {
			t.Parallel()

			var prompts []string

			session := newApprovingSession(t, newTestTree(t), "accept", &prompts)

			res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      tool,
				Arguments: map[string]any{"parallelism": -1},
			})
			require.NoError(t, err)

			assert.True(t, res.IsError, "the run must be refused")
			assert.Empty(t, prompts, "nobody may be asked to approve a run that cannot happen")
		})
	}
}

// TestApprovalShowsTheRunArguments pins that the command line a person
// approves names the filters and the parallelism the run will use, with each
// filter quoted as one shell word.
func TestApprovalShowsTheRunArguments(t *testing.T) {
	t.Parallel()

	var prompts []string

	session := newApprovingSession(t, newTestTree(t), "decline", &prompts)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "apply",
		Arguments: map[string]any{
			"filter":      []any{"./a", "!./b"},
			"parallelism": 2,
		},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError, "a declined run must not report success")

	require.Len(t, prompts, 1)
	assert.Contains(
		t,
		prompts[0],
		`terragrunt run --all apply --filter=./a --filter='!./b' --parallelism=2`,
	)
}
