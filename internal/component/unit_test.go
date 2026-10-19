package component_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/stretchr/testify/assert"
)

// TestUnitOutputFileSeparatesGitReferences pins that the same unit discovered
// in worktrees of two Git references gets a plan file of its own for each,
// and that a unit discovered in the working tree keeps the stack's layout.
func TestUnitOutputFileSeparatesGitReferences(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	root := filepath.Join(base, "repo")
	out := filepath.Join(base, "out")

	unitAt := func(workingDir, ref string) *component.Unit {
		unit := component.NewUnit(filepath.Join(workingDir, "env", "app"))
		unit.SetDiscoveryContext(&component.DiscoveryContext{WorkingDir: workingDir, Ref: ref})

		return unit
	}

	tests := []struct {
		unit *component.Unit
		name string
		want string
	}{
		{
			name: "working tree",
			unit: unitAt(root, ""),
			want: filepath.Join(out, "env", "app", "tfplan.tfplan"),
		},
		{
			name: "from reference",
			unit: unitAt(filepath.Join(base, "worktree-a"), "HEAD~1"),
			want: filepath.Join(out, "@HEAD_1", "env", "app", "tfplan.tfplan"),
		},
		{
			name: "to reference",
			unit: unitAt(filepath.Join(base, "worktree-b"), "feature/x"),
			want: filepath.Join(out, "@feature_x", "env", "app", "tfplan.tfplan"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.unit.OutputFile(root, out))
			assert.Equal(
				t,
				filepath.Join(filepath.Dir(tc.want), "tfplan.json"),
				tc.unit.OutputJSONFile(root, out),
			)
		})
	}
}
