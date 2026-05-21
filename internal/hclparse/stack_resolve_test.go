package hclparse_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscoverStackChildUnits_NestedSourceResolution verifies discovery populates ChildRefs only when the source expression resolves against the default (stdlib-only) discovery eval context.
func TestDiscoverStackChildUnits_NestedSourceResolution(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		sourceExpr          string
		nestedStackDir      string
		expectChildResolved bool
	}{
		{
			name:                "plain string literal resolves",
			sourceExpr:          `"../child-stack"`,
			nestedStackDir:      "/abs/child-stack",
			expectChildResolved: true,
		},
		{
			name:                "absolute literal resolves",
			sourceExpr:          `"/abs/child-stack"`,
			nestedStackDir:      "/abs/child-stack",
			expectChildResolved: true,
		},
		{
			name:                "format function (tf stdlib) resolves",
			sourceExpr:          `format("%s/%s", "/abs", "child-stack")`,
			nestedStackDir:      "/abs/child-stack",
			expectChildResolved: true,
		},
		{
			name:                "replace function (tf stdlib) resolves",
			sourceExpr:          `replace("/abs/CHILD-stack", "CHILD", "child")`,
			nestedStackDir:      "/abs/child-stack",
			expectChildResolved: true,
		},
		{
			name:                "terragrunt function not in stdlib skips recursion",
			sourceExpr:          `"${get_terragrunt_dir()}/../child"`,
			expectChildResolved: false,
		},
		{
			name:                "local namespace skips recursion",
			sourceExpr:          `"${local.x}/child"`,
			expectChildResolved: false,
		},
		{
			name:                "unit namespace skips recursion",
			sourceExpr:          `unit.foo.path`,
			expectChildResolved: false,
		},
		{
			name:                "values namespace skips recursion",
			sourceExpr:          `"${values.cloud}-child"`,
			expectChildResolved: false,
		},
		{
			name:                "null literal skips recursion",
			sourceExpr:          `null`,
			expectChildResolved: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()
			parentDir := "/abs/parent"

			require.NoError(t, fs.MkdirAll(parentDir, 0755))
			require.NoError(t, vfs.WriteFile(fs,
				parentDir+"/terragrunt.stack.hcl",
				[]byte(fmt.Sprintf("stack \"child\" {\n  source = %s\n  path   = \"child\"\n}\n", tc.sourceExpr)),
				0644))

			if tc.nestedStackDir != "" {
				require.NoError(t, fs.MkdirAll(tc.nestedStackDir, 0755))
				require.NoError(t, vfs.WriteFile(fs,
					tc.nestedStackDir+"/terragrunt.stack.hcl",
					[]byte("unit \"vpc\" {\n  source = \"../units/vpc\"\n  path   = \"vpc\"\n}\n"),
					0644))
			}

			refs := hclparse.DiscoverStackChildUnits(fs, parentDir, "/gen/parent")
			require.Len(t, refs, 1)
			assert.Equal(t, "child", refs[0].Name)

			if tc.expectChildResolved {
				require.Len(t, refs[0].ChildRefs, 1, "child stack source should have been resolved and recursion should have populated ChildRefs")
				assert.Equal(t, "vpc", refs[0].ChildRefs[0].Name)

				return
			}

			assert.Empty(t, refs[0].ChildRefs, "child stack source must NOT be resolvable so recursion is skipped")
		})
	}
}
