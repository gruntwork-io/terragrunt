package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorRule is a retry block and an ignore block with the same label. They match the errors "retry-<letter>" and
// "ignore-<letter>" for the first rank letters of the alphabet, so the error for the nth letter matches every rule
// ranked n or higher.
type errorRule struct {
	label       string
	rank        int
	maxAttempts int
}

func TestParseConfigMatchesErrorsBlocksInDeclarationOrder(t *testing.T) {
	t.Parallel()

	oneFile := []errorRule{
		{label: "throttle", rank: 1, maxAttempts: 2},
		{label: "dns", rank: 2, maxAttempts: 3},
		{label: "quota", rank: 3, maxAttempts: 4},
		{label: "lock", rank: 4, maxAttempts: 5},
		{label: "auth", rank: 5, maxAttempts: 6},
		{label: "network", rank: 6, maxAttempts: 7},
		{label: "state", rank: 7, maxAttempts: 8},
		{label: "registry", rank: 8, maxAttempts: 9},
	}

	repeatedLabel := []errorRule{
		{label: "quota", rank: 3, maxAttempts: 9},
		{label: "throttle", rank: 2, maxAttempts: 2},
		{label: "dns", rank: 3, maxAttempts: 3},
		{label: "quota", rank: 1, maxAttempts: 4},
	}

	wantRepeatedLabel := []errorRule{
		{label: "quota", rank: 1, maxAttempts: 4},
		{label: "throttle", rank: 2, maxAttempts: 2},
		{label: "dns", rank: 3, maxAttempts: 3},
	}

	tests := []struct {
		name    string
		include string
		parent  []errorRule
		child   []errorRule
		want    []errorRule
	}{
		{
			name:  "one file",
			child: oneFile,
			want:  oneFile,
		},
		{
			name:  "one file where a later block reuses a label",
			child: repeatedLabel,
			want:  wantRepeatedLabel,
		},
		{
			name:    "deep include where a later block of the including file reuses a label",
			include: `merge_strategy = "deep"`,
			child:   repeatedLabel,
			want:    wantRepeatedLabel,
		},
		{
			name:    "deep include where a later block of the included file reuses a label",
			include: `merge_strategy = "deep"`,
			parent:  repeatedLabel,
			want:    wantRepeatedLabel,
		},
		{
			name:    "deep include where the included file reuses a label and the including file adds a block",
			include: `merge_strategy = "deep"`,
			parent:  repeatedLabel,
			child:   []errorRule{{label: "state", rank: 4, maxAttempts: 6}},
			want: []errorRule{
				{label: "quota", rank: 1, maxAttempts: 4},
				{label: "throttle", rank: 2, maxAttempts: 2},
				{label: "dns", rank: 3, maxAttempts: 3},
				{label: "state", rank: 4, maxAttempts: 6},
			},
		},
		{
			name:    "deep include where the including file reuses a label of the included file twice",
			include: `merge_strategy = "deep"`,
			parent: []errorRule{
				{label: "throttle", rank: 1, maxAttempts: 2},
				{label: "quota", rank: 2, maxAttempts: 3},
			},
			child: []errorRule{
				{label: "quota", rank: 4, maxAttempts: 9},
				{label: "dns", rank: 3, maxAttempts: 4},
				{label: "quota", rank: 2, maxAttempts: 5},
			},
			want: []errorRule{
				{label: "throttle", rank: 1, maxAttempts: 2},
				{label: "quota", rank: 2, maxAttempts: 5},
				{label: "dns", rank: 3, maxAttempts: 4},
			},
		},
		{
			name:    "deep include keeps the parent order and appends the child's new blocks",
			include: `merge_strategy = "deep"`,
			parent: []errorRule{
				{label: "throttle", rank: 1, maxAttempts: 2},
				{label: "dns", rank: 2, maxAttempts: 3},
				{label: "quota", rank: 3, maxAttempts: 4},
				{label: "lock", rank: 4, maxAttempts: 5},
				{label: "auth", rank: 5, maxAttempts: 6},
				{label: "network", rank: 6, maxAttempts: 7},
			},
			child: []errorRule{
				{label: "state", rank: 7, maxAttempts: 8},
				{label: "quota", rank: 3, maxAttempts: 20},
				{label: "registry", rank: 8, maxAttempts: 9},
				{label: "checksum", rank: 9, maxAttempts: 10},
				{label: "backend", rank: 10, maxAttempts: 11},
				{label: "plugin", rank: 11, maxAttempts: 12},
			},
			want: []errorRule{
				{label: "throttle", rank: 1, maxAttempts: 2},
				{label: "dns", rank: 2, maxAttempts: 3},
				{label: "quota", rank: 3, maxAttempts: 20},
				{label: "lock", rank: 4, maxAttempts: 5},
				{label: "auth", rank: 5, maxAttempts: 6},
				{label: "network", rank: 6, maxAttempts: 7},
				{label: "state", rank: 7, maxAttempts: 8},
				{label: "registry", rank: 8, maxAttempts: 9},
				{label: "checksum", rank: 9, maxAttempts: 10},
				{label: "backend", rank: 10, maxAttempts: 11},
				{label: "plugin", rank: 11, maxAttempts: 12},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			childHCL := errorsBlockHCL(tt.child)

			if tt.include != "" {
				parentPath := filepath.Join(tmpDir, "root.hcl")
				require.NoError(t, os.WriteFile(parentPath, []byte(errorsBlockHCL(tt.parent)), 0644))

				childHCL += `
include "root" {
  path = "` + filepath.ToSlash(parentPath) + `"
  ` + tt.include + `
}
`
			}

			childDir := filepath.Join(tmpDir, "unit")
			require.NoError(t, os.MkdirAll(childDir, 0755))

			childPath := filepath.Join(childDir, config.DefaultTerragruntConfigPath)
			require.NoError(t, os.WriteFile(childPath, []byte(childHCL), 0644))

			v := venvtest.NewWithOSFS()
			ctx, pctx := newTestParsingContext(t, childPath)
			l := logger.CreateLogger()

			parsed, err := config.ParseConfigFile(ctx, l, v, pctx, childPath, nil)
			require.NoError(t, err)

			fromConfig, err := parsed.ErrorsConfig()
			require.NoError(t, err)

			fromRunConfig, err := parsed.ToRunConfig(l, v.FS).ErrorsConfig()
			require.NoError(t, err)

			partialCtx, partialPctx := newTestParsingContext(t, childPath)

			partial, err := config.PartialParseConfigFile(
				partialCtx, l, v, partialPctx.WithDecodeList(config.ErrorsBlock), childPath, nil,
			)
			require.NoError(t, err)

			fromPartial, err := partial.ErrorsConfig()
			require.NoError(t, err)

			for _, errorsConfig := range []*errorconfig.Config{fromConfig, fromRunConfig, fromPartial} {
				for _, want := range tt.want {
					letter := string(rune('a' + want.rank - 1))

					action, err := errorsConfig.AttemptErrorRecovery(l, errors.New("retry-"+letter), 1)
					require.NoError(t, err)
					require.NotNil(t, action)
					assert.Equal(t, want.label, action.RetryBlockName)
					assert.Equal(t, want.maxAttempts, action.RetryAttempts)

					action, err = errorsConfig.AttemptErrorRecovery(l, errors.New("ignore-"+letter), 1)
					require.NoError(t, err)
					require.NotNil(t, action)
					assert.Equal(t, want.label, action.IgnoreBlockName)
				}
			}
		})
	}
}

func errorsBlockHCL(rules []errorRule) string {
	if len(rules) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("errors {\n")

	for _, rule := range rules {
		lastLetter := string(rune('a' + rule.rank - 1))

		fmt.Fprintf(&b, `
  retry %[1]q {
    retryable_errors   = ["retry-[a-%[2]s]"]
    max_attempts       = %[3]d
    sleep_interval_sec = 0
  }

  ignore %[1]q {
    ignorable_errors = ["ignore-[a-%[2]s]"]
  }
`, rule.label, lastLetter, rule.maxAttempts)
	}

	b.WriteString("}\n")

	return b.String()
}
