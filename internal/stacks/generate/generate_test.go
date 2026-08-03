package generate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateStacksCyclicSource_FailsAtMaxLevel pins the cycle detection in
// GenerateStacks: a stack whose source resolves to a stack that sources itself
// nests one generation level per iteration, and the run must stop with a cycle
// error once the configured level cap is reached.
func TestGenerateStacksCyclicSource_FailsAtMaxLevel(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	liveDir := filepath.Join(tmpDir, "live")
	stackDir := filepath.Join(tmpDir, "stack")

	// Both stack files source the same absolute stack directory, so every
	// generated level discovers one more copy of the self-sourcing stack.
	stackConfig := fmt.Sprintf(`stack "stack" {
  source = %q
  path   = "stack"
}
`, stackDir)

	require.NoError(t, os.MkdirAll(liveDir, 0o755))
	require.NoError(t, os.MkdirAll(stackDir, 0o755))
	require.NoError(
		t,
		os.WriteFile(filepath.Join(liveDir, "terragrunt.stack.hcl"), []byte(stackConfig), 0o644),
	)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(stackConfig), 0o644),
	)

	l := logger.CreateLogger()

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = liveDir
	opts.RootWorkingDir = liveDir
	opts.Parallelism = 1
	opts.NoCAS = true

	const maxLevel = 5

	err := generate.NewGenerator().
		WithMaxLevel(maxLevel).
		GenerateStacks(t.Context(), l, venv.OSVenv(), opts, nil)

	require.ErrorContains(
		t,
		err,
		fmt.Sprintf("cycle detected: maximum level (%d) exceeded", maxLevel),
		"self-sourcing stacks must trip the nesting cap instead of recursing until the filesystem gives up",
	)
}

// TestGeneratorDefaultMaxLevel pins the default nesting cap so it stays
// generous enough that real stack trees never trip cycle detection.
func TestGeneratorDefaultMaxLevel(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		1024,
		generate.DefaultMaxLevel,
		"default level cap must stay generous so only genuine cycles reach it",
	)
}
