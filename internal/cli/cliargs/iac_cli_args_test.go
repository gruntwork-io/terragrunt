package cliargs_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/cliargs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     []string
		wantCmd   string
		wantFlags []string
		wantArgs  []string
	}{
		{
			name:      "simple apply",
			input:     []string{"apply", "tfplan"},
			wantCmd:   "apply",
			wantFlags: nil,
			wantArgs:  []string{"tfplan"},
		},
		{
			name:      "apply with flags",
			input:     []string{"apply", "-input=false", "tfplan"},
			wantCmd:   "apply",
			wantFlags: []string{"-input=false"},
			wantArgs:  []string{"tfplan"},
		},
		{
			name:      "issue #5409 - plan file before flags",
			input:     []string{"apply", "tfplan", "-input=false", "-auto-approve"},
			wantCmd:   "apply",
			wantFlags: []string{"-input=false", "-auto-approve"},
			wantArgs:  []string{"tfplan"},
		},
		{
			name:      "destroy case with plan file in middle",
			input:     []string{"apply", "-destroy", "/tmp/x.tfplan", "-auto-approve"},
			wantCmd:   "apply",
			wantFlags: []string{"-destroy", "-auto-approve"},
			wantArgs:  []string{"/tmp/x.tfplan"},
		},
		{
			name:      "full destroy case with all flags",
			input:     []string{"apply", "-no-color", "-destroy", "-input=false", "/tmp/plan.tfplan", "-auto-approve"},
			wantCmd:   "apply",
			wantFlags: []string{"-no-color", "-destroy", "-input=false", "-auto-approve"},
			wantArgs:  []string{"/tmp/plan.tfplan"},
		},
		{
			name:      "var with space-separated value",
			input:     []string{"plan", "-var", "key=value", "-out=myplan"},
			wantCmd:   "plan",
			wantFlags: []string{"-var", "key=value", "-out=myplan"},
			wantArgs:  nil,
		},
		{
			name:      "target with space-separated value",
			input:     []string{"plan", "-target", "module.foo", "tfplan"},
			wantCmd:   "plan",
			wantFlags: []string{"-target", "module.foo"},
			wantArgs:  []string{"tfplan"},
		},
		{
			name:      "var-file with space-separated value",
			input:     []string{"apply", "-var-file", "vars.tfvars"},
			wantCmd:   "apply",
			wantFlags: []string{"-var-file", "vars.tfvars"},
			wantArgs:  nil,
		},
		{
			name:      "empty args",
			input:     []string{},
			wantCmd:   "",
			wantFlags: nil,
			wantArgs:  nil,
		},
		{
			name:      "only command",
			input:     []string{"apply"},
			wantCmd:   "apply",
			wantFlags: nil,
			wantArgs:  nil,
		},
		{
			name:      "only flags",
			input:     []string{"-auto-approve"},
			wantCmd:   "",
			wantFlags: []string{"-auto-approve"},
			wantArgs:  nil,
		},
		{
			name:      "multiple positional args",
			input:     []string{"apply", "plan1", "plan2", "-auto-approve"},
			wantCmd:   "apply",
			wantFlags: []string{"-auto-approve"},
			wantArgs:  []string{"plan1", "plan2"},
		},
		{
			name:      "var with equals format",
			input:     []string{"plan", "-var=key=value", "tfplan"},
			wantCmd:   "plan",
			wantFlags: []string{"-var=key=value"},
			wantArgs:  []string{"tfplan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cliargs.Parse(tt.input)

			assert.Equal(t, tt.wantCmd, got.Command)

			if tt.wantFlags == nil {
				assert.Empty(t, got.Flags)
			} else {
				assert.Equal(t, tt.wantFlags, got.Flags)
			}

			if tt.wantArgs == nil {
				assert.Empty(t, got.Arguments)
			} else {
				assert.Equal(t, tt.wantArgs, got.Arguments)
			}
		})
	}
}

func TestToArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input *cliargs.IacCliArgs
		want  []string
	}{
		{
			name: "basic apply with flags and args",
			input: &cliargs.IacCliArgs{
				Command:   "apply",
				Flags:     []string{"-input=false", "-auto-approve"},
				Arguments: []string{"tfplan"},
			},
			want: []string{"apply", "-input=false", "-auto-approve", "tfplan"},
		},
		{
			name: "command only",
			input: &cliargs.IacCliArgs{
				Command: "plan",
			},
			want: []string{"plan"},
		},
		{
			name: "empty",
			input: &cliargs.IacCliArgs{
				Flags:     []string{},
				Arguments: []string{},
			},
			want: []string{},
		},
		{
			name: "flags only",
			input: &cliargs.IacCliArgs{
				Flags: []string{"-auto-approve"},
			},
			want: []string{"-auto-approve"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.input.ToArgs()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseToArgsRoundTrip(t *testing.T) {
	t.Parallel()

	// Test that Parse followed by ToArgs produces correct output
	// (not necessarily the same input, but correctly ordered output)
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "issue #5409 - reorder plan file to end",
			input: []string{"apply", "tfplan", "-input=false", "-auto-approve"},
			want:  []string{"apply", "-input=false", "-auto-approve", "tfplan"},
		},
		{
			name:  "already correct order",
			input: []string{"apply", "-input=false", "-auto-approve", "tfplan"},
			want:  []string{"apply", "-input=false", "-auto-approve", "tfplan"},
		},
		{
			name:  "destroy with plan file in middle",
			input: []string{"apply", "-destroy", "/tmp/plan.tfplan", "-auto-approve"},
			want:  []string{"apply", "-destroy", "-auto-approve", "/tmp/plan.tfplan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parsed := cliargs.Parse(tt.input)
			got := parsed.ToArgs()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAddFlag(t *testing.T) {
	t.Parallel()

	args := &cliargs.IacCliArgs{
		Command: "apply",
		Flags:   []string{"-auto-approve"},
	}

	// Add new flag
	args.AddFlag("-input=false")
	assert.Contains(t, args.Flags, "-input=false")

	// Adding duplicate should not add again
	args.AddFlag("-auto-approve")
	count := 0

	for _, f := range args.Flags {
		if f == "-auto-approve" {
			count++
		}
	}

	assert.Equal(t, 1, count)
}

func TestHasFlag(t *testing.T) {
	t.Parallel()

	args := &cliargs.IacCliArgs{
		Flags: []string{"-auto-approve", "-input=false"},
	}

	assert.True(t, args.HasFlag("-auto-approve"))
	assert.True(t, args.HasFlag("-input"))
	assert.False(t, args.HasFlag("-destroy"))
}

func TestRemoveFlag(t *testing.T) {
	t.Parallel()

	args := &cliargs.IacCliArgs{
		Flags: []string{"-auto-approve", "-input=false", "-destroy"},
	}

	args.RemoveFlag("-input")
	require.Len(t, args.Flags, 2)
	assert.Equal(t, []string{"-auto-approve", "-destroy"}, args.Flags)

	args.RemoveFlag("-auto-approve")
	require.Len(t, args.Flags, 1)
	assert.Equal(t, []string{"-destroy"}, args.Flags)
}

func TestAddArgument(t *testing.T) {
	t.Parallel()

	args := &cliargs.IacCliArgs{
		Command:   "apply",
		Arguments: []string{"plan1"},
	}

	args.AddArgument("plan2")
	assert.Equal(t, []string{"plan1", "plan2"}, args.Arguments)

	// Adding duplicate should not add again
	args.AddArgument("plan1")
	assert.Equal(t, []string{"plan1", "plan2"}, args.Arguments)
}
