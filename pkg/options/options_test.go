package options

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
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
	}

	for _, tt := range tc {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := &TerragruntOptions{
				TerraformCliArgs: clihelper.NewIacArgs(tt.initial...),
			}
			opts.InsertTerraformCliArgs(tt.insert...)
			assert.Equal(t, tt.expected, opts.TerraformCliArgs.Slice())
		})
	}
}

func TestInsertTerraformCliArgsNilGuard(t *testing.T) {
	t.Parallel()

	opts := &TerragruntOptions{}
	// Should not panic
	opts.InsertTerraformCliArgs("plan")
	assert.Equal(t, []string{"plan"}, opts.TerraformCliArgs.Slice())
}
