package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
)

func TestInsertTerraformCliArgsSubcommandReplacement(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		initial  []string
		insert   []string
		expected []string
	}{
		{
			name:     "replace_lock_with_mirror",
			initial:  []string{"providers", "lock", "-platform=linux_amd64"},
			insert:   []string{"providers", "mirror"},
			expected: []string{"providers", "mirror", "-platform=linux_amd64"},
		},
		{
			name:     "no_replacement_if_no_subcommand",
			initial:  []string{"apply", "-auto-approve"},
			insert:   []string{"-var", "foo=bar"},
			expected: []string{"apply", "-var", "foo=bar", "-auto-approve"},
		},
		{
			name:     "append_new_subcommand",
			initial:  []string{"state"},
			insert:   []string{"list"},
			expected: []string{"state", "list"},
		},
		{
			name:     "same_command_no_change",
			initial:  []string{"apply", "-auto-approve"},
			insert:   []string{"apply"},
			expected: []string{"apply", "-auto-approve"},
		},
		{
			name:     "unknown_command_becomes_argument",
			initial:  []string{"apply", "-auto-approve"},
			insert:   []string{"myplan.tfplan"},
			expected: []string{"apply", "-auto-approve", "myplan.tfplan"},
		},
		{
			name:     "empty_insert_no_change",
			initial:  []string{"plan", "-out=plan.tfplan"},
			insert:   []string{},
			expected: []string{"plan", "-out=plan.tfplan"},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := &options.TerragruntOptions{
				TerraformCliArgs: iacargs.New(tt.initial...),
			}
			opts.InsertTerraformCliArgs(tt.insert...)
			assert.Equal(t, tt.expected, opts.TerraformCliArgs.Slice())
		})
	}
}

func TestInsertTerraformCliArgsNilGuard(t *testing.T) {
	t.Parallel()

	opts := &options.TerragruntOptions{}
	// Should not panic
	opts.InsertTerraformCliArgs("plan")
	assert.Equal(t, []string{"plan"}, opts.TerraformCliArgs.Slice())
}
