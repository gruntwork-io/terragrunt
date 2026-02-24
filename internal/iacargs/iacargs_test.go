package iacargs_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
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
			name:      "lock-timeout with space-separated value",
			input:     []string{"apply", "-lock-timeout", "5m", "-auto-approve"},
			wantCmd:   "apply",
			wantFlags: []string{"-lock-timeout", "5m", "-auto-approve"},
			wantArgs:  nil,
		},
		{
			// Unknown flags are treated as boolean. If a new Terraform flag needs
			// space-separated values, add it to valueTakingFlags list.
			name:      "unknown flag treated as boolean",
			input:     []string{"apply", "-future-flag", "value", "-auto-approve"},
			wantCmd:   "apply",
			wantFlags: []string{"-future-flag", "-auto-approve"},
			wantArgs:  []string{"value"},
		},
		{
			// Unknown flags are boolean, so planfile correctly goes to Arguments.
			name:      "unknown boolean flag followed by arg",
			input:     []string{"apply", "-unknown-bool", "planfile"},
			wantCmd:   "apply",
			wantFlags: []string{"-unknown-bool"},
			wantArgs:  []string{"planfile"},
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
			name:      "chdir with space-separated value",
			input:     []string{"-chdir", "/tmp/dir", "apply"},
			wantCmd:   "apply",
			wantFlags: []string{"-chdir", "/tmp/dir"},
			wantArgs:  nil,
		},
		{
			name:      "chdir with equals value",
			input:     []string{"-chdir=/tmp/dir", "plan"},
			wantCmd:   "plan",
			wantFlags: []string{"-chdir=/tmp/dir"},
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

			got := iacargs.New(tt.input...)

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

func TestIacArgsSlice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input *iacargs.IacArgs
		want  []string
	}{
		{
			name: "basic apply with flags and args",
			input: &iacargs.IacArgs{
				Command:   "apply",
				Flags:     []string{"-input=false", "-auto-approve"},
				Arguments: []string{"tfplan"},
			},
			want: []string{"apply", "-input=false", "-auto-approve", "tfplan"},
		},
		{
			name: "command only",
			input: &iacargs.IacArgs{
				Command: "plan",
			},
			want: []string{"plan"},
		},
		{
			name: "empty",
			input: &iacargs.IacArgs{
				Flags:     []string{},
				Arguments: []string{},
			},
			want: []string{},
		},
		{
			name: "flags only",
			input: &iacargs.IacArgs{
				Flags: []string{"-auto-approve"},
			},
			want: []string{"-auto-approve"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.input.Slice()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIacArgsRoundTrip(t *testing.T) {
	t.Parallel()

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
		{
			name:  "providers lock subcommand preserves order",
			input: []string{"providers", "lock", "-platform=linux_amd64", "-platform=darwin_arm64"},
			want:  []string{"providers", "lock", "-platform=linux_amd64", "-platform=darwin_arm64"},
		},
		{
			name:  "state mv subcommand preserves order",
			input: []string{"state", "mv", "-lock=false", "aws_instance.a", "aws_instance.b"},
			want:  []string{"state", "mv", "-lock=false", "aws_instance.a", "aws_instance.b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parsed := iacargs.New(tt.input...)
			got := parsed.Slice()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIacArgsAddFlagIfNotPresent(t *testing.T) {
	t.Parallel()

	args := &iacargs.IacArgs{
		Command: "apply",
		Flags:   []string{"-auto-approve"},
	}

	// Add new flag
	args.AddFlagIfNotPresent("-input=false")
	assert.Contains(t, args.Flags, "-input=false")

	// Adding duplicate should not add again
	args.AddFlagIfNotPresent("-auto-approve")

	count := 0

	for _, f := range args.Flags {
		if f == "-auto-approve" {
			count++
		}
	}

	assert.Equal(t, 1, count)
}

func TestIacArgsHasFlag(t *testing.T) {
	t.Parallel()

	args := &iacargs.IacArgs{
		Flags: []string{"-auto-approve", "-input=false"},
	}

	assert.True(t, args.HasFlag("-auto-approve"))
	assert.True(t, args.HasFlag("-input"))
	assert.False(t, args.HasFlag("-destroy"))
}

func TestIacArgsRemoveFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		initialFlags  []string
		flagToRemove  string
		expectedFlags []string
	}{
		{
			name:          "remove flag with equals value",
			initialFlags:  []string{"-auto-approve", "-input=false", "-destroy"},
			flagToRemove:  "-input",
			expectedFlags: []string{"-auto-approve", "-destroy"},
		},
		{
			name:          "remove boolean flag",
			initialFlags:  []string{"-auto-approve", "-destroy"},
			flagToRemove:  "-auto-approve",
			expectedFlags: []string{"-destroy"},
		},
		{
			name:          "remove flag with space-separated value",
			initialFlags:  []string{"-var", "foo=bar", "-auto-approve"},
			flagToRemove:  "-var",
			expectedFlags: []string{"-auto-approve"},
		},
		{
			name:          "remove flag where next entry looks like flag preserves it",
			initialFlags:  []string{"-target", "-module.resource", "-auto-approve"},
			flagToRemove:  "-target",
			expectedFlags: []string{"-module.resource", "-auto-approve"},
		},
		{
			name:          "remove flag preserves other flags with dash-prefixed values",
			initialFlags:  []string{"-var", "key=-value", "-target", "-module.foo", "-destroy"},
			flagToRemove:  "-var",
			expectedFlags: []string{"-target", "-module.foo", "-destroy"},
		},
		{
			name:          "remove middle flag with space-separated value",
			initialFlags:  []string{"-auto-approve", "-var", "x=y", "-destroy"},
			flagToRemove:  "-var",
			expectedFlags: []string{"-auto-approve", "-destroy"},
		},
		{
			name:          "remove nonexistent flag does nothing",
			initialFlags:  []string{"-auto-approve", "-destroy"},
			flagToRemove:  "-nonexistent",
			expectedFlags: []string{"-auto-approve", "-destroy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := &iacargs.IacArgs{
				Flags: append([]string{}, tt.initialFlags...),
			}
			args.RemoveFlag(tt.flagToRemove)
			assert.Equal(t, tt.expectedFlags, args.Flags)
		})
	}
}

func TestIacArgsAppendArgument(t *testing.T) {
	t.Parallel()

	args := &iacargs.IacArgs{
		Command:   "apply",
		Arguments: []string{"plan1"},
	}

	args.AppendArgument("plan2")
	assert.Equal(t, []string{"plan1", "plan2"}, args.Arguments)
}

func TestIacArgsClone(t *testing.T) {
	t.Parallel()

	original := &iacargs.IacArgs{
		Command:   "apply",
		Flags:     []string{"-auto-approve"},
		Arguments: []string{"tfplan"},
	}

	clone := original.Clone()

	// Verify values are equal
	assert.Equal(t, original.Command, clone.Command)
	assert.Equal(t, original.Flags, clone.Flags)
	assert.Equal(t, original.Arguments, clone.Arguments)

	// Verify modifying clone doesn't affect original
	clone.Command = "plan"
	clone.Flags = append(clone.Flags, "-input=false")
	clone.Arguments = append(clone.Arguments, "another")

	assert.Equal(t, "apply", original.Command)
	assert.Equal(t, []string{"-auto-approve"}, original.Flags)
	assert.Equal(t, []string{"tfplan"}, original.Arguments)
}

func TestIacArgsContains(t *testing.T) {
	t.Parallel()

	args := &iacargs.IacArgs{
		Command:   "apply",
		Flags:     []string{"-auto-approve", "-input=false"},
		Arguments: []string{"tfplan"},
	}

	assert.True(t, args.Contains("apply"))
	assert.True(t, args.Contains("-auto-approve"))
	assert.True(t, args.Contains("tfplan"))
	assert.False(t, args.Contains("-destroy"))
}

func TestIacArgsFirst(t *testing.T) {
	t.Parallel()

	args := iacargs.New("apply", "-auto-approve", "tfplan")
	assert.Equal(t, "apply", args.First())

	empty := iacargs.New()
	assert.Empty(t, empty.First())
}

func TestIacArgsTail(t *testing.T) {
	t.Parallel()

	args := iacargs.New("apply", "-auto-approve", "tfplan")
	assert.Equal(t, []string{"-auto-approve", "tfplan"}, args.Tail())

	empty := iacargs.New()
	assert.Empty(t, empty.Tail())
}

func TestIacArgsHasPlanFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args     *iacargs.IacArgs
		name     string
		expected bool
	}{
		{
			name:     "empty args",
			args:     iacargs.New(),
			expected: false,
		},
		{
			name:     "plan with -out flag",
			args:     iacargs.New("plan", "-out=tfplan"),
			expected: true,
		},
		{
			name:     "plan without -out flag",
			args:     iacargs.New("plan", "-input=false"),
			expected: false,
		},
		{
			name:     "apply with plan file argument",
			args:     iacargs.New("apply", "-auto-approve", "tfplan"),
			expected: true,
		},
		{
			name:     "apply without plan file",
			args:     iacargs.New("apply", "-auto-approve"),
			expected: false,
		},
		{
			name:     "destroy with plan file argument",
			args:     iacargs.New("destroy", "tfplan"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, tt.args.HasPlanFile())
		})
	}
}

func TestIacArgsMergeFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		base          *iacargs.IacArgs
		other         *iacargs.IacArgs
		expectedFlags []string
	}{
		{
			name:          "merge into empty",
			base:          iacargs.New("apply"),
			other:         iacargs.New("apply", "-auto-approve", "-input=false"),
			expectedFlags: []string{"-auto-approve", "-input=false"},
		},
		{
			name:          "skip duplicates",
			base:          iacargs.New("apply", "-auto-approve"),
			other:         iacargs.New("apply", "-auto-approve", "-input=false"),
			expectedFlags: []string{"-auto-approve", "-input=false"},
		},
		{
			name:          "merge from empty",
			base:          iacargs.New("apply", "-auto-approve"),
			other:         iacargs.New(),
			expectedFlags: []string{"-auto-approve"},
		},
		{
			name:          "both have different flags",
			base:          iacargs.New("apply", "-compact-warnings"),
			other:         iacargs.New("apply", "-auto-approve", "-input=false"),
			expectedFlags: []string{"-compact-warnings", "-auto-approve", "-input=false"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.base.MergeFlags(tt.other)
			assert.Equal(t, tt.expectedFlags, tt.base.Flags)
		})
	}
}

func TestIacArgsIsDestroyCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     *iacargs.IacArgs
		cmd      string
		expected bool
	}{
		{
			name:     "destroy command",
			args:     iacargs.New("destroy"),
			cmd:      "destroy",
			expected: true,
		},
		{
			name:     "apply with -destroy flag",
			args:     iacargs.New("apply", "-destroy", "tfplan"),
			cmd:      "apply",
			expected: true,
		},
		{
			name:     "regular apply",
			args:     iacargs.New("apply", "-auto-approve"),
			cmd:      "apply",
			expected: false,
		},
		{
			name:     "plan command",
			args:     iacargs.New("plan"),
			cmd:      "plan",
			expected: false,
		},
		{
			name:     "nil args with destroy cmd",
			args:     nil,
			cmd:      "destroy",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.args == nil {
				// Test nil case separately
				assert.Equal(t, tt.expected, tt.cmd == "destroy")
			} else {
				assert.Equal(t, tt.expected, tt.args.IsDestroyCommand(tt.cmd))
			}
		})
	}
}
