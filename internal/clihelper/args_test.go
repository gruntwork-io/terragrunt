package clihelper_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockArgs = func() clihelper.Args { return clihelper.Args{"one", "-foo", "two", "--bar", "value"} }

func TestArgsSlice(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Slice()
	expected := []string(mockArgs())
	assert.Equal(t, expected, actual)
}

func TestArgsTail(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Tail()
	expected := mockArgs()[1:]
	assert.Equal(t, expected, actual)
}

func TestArgsFirst(t *testing.T) {
	t.Parallel()

	actual := mockArgs().First()
	expected := mockArgs()[0]
	assert.Equal(t, expected, actual)
}

func TestArgsGet(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Get(2)
	expected := "two"
	assert.Equal(t, expected, actual)
}

func TestArgsLen(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Len()
	expected := 5
	assert.Equal(t, expected, actual)
}

func TestArgsPresent(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Present()
	expected := true
	assert.Equal(t, expected, actual)

	args := clihelper.Args([]string{})
	actual = args.Present()
	expected = false
	assert.Equal(t, expected, actual)
}

func TestArgsCommandName(t *testing.T) {
	t.Parallel()

	actual := mockArgs().CommandName()
	expected := "one"
	assert.Equal(t, expected, actual)

	args := mockArgs()[1:]
	actual = args.CommandName()
	expected = "two"
	assert.Equal(t, expected, actual)
}

func TestArgsNormalize(t *testing.T) {
	t.Parallel()

	actual := mockArgs().Normalize(clihelper.SingleDashFlag).Slice()
	expected := []string{"one", "-foo", "two", "-bar", "value"}
	assert.Equal(t, expected, actual)

	actual = mockArgs().Normalize(clihelper.DoubleDashFlag).Slice()
	expected = []string{"one", "--foo", "two", "--bar", "value"}
	assert.Equal(t, expected, actual)
}

func TestArgsRemove(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		args           clihelper.Args
		expectedArgs   clihelper.Args
		removeName     string
		expectedResult clihelper.Args
	}{
		{
			mockArgs(),
			mockArgs(),
			"two",
			clihelper.Args{"one", "-foo", "--bar", "value"},
		},
		{
			mockArgs(),
			mockArgs(),
			"one",
			clihelper.Args{"-foo", "two", "--bar", "value"},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := tc.args.Remove(tc.removeName)
			assert.Equal(t, tc.expectedResult, actual)
			assert.Equal(t, tc.expectedArgs, tc.args)
		})
	}
}

// IacArgs tests

func TestNewIacArgs(t *testing.T) {
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

			got := clihelper.NewIacArgs(tt.input...)

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
		input *clihelper.IacArgs
		want  []string
	}{
		{
			name: "basic apply with flags and args",
			input: &clihelper.IacArgs{
				Command:   "apply",
				Flags:     []string{"-input=false", "-auto-approve"},
				Arguments: []string{"tfplan"},
			},
			want: []string{"apply", "-input=false", "-auto-approve", "tfplan"},
		},
		{
			name: "command only",
			input: &clihelper.IacArgs{
				Command: "plan",
			},
			want: []string{"plan"},
		},
		{
			name: "empty",
			input: &clihelper.IacArgs{
				Flags:     []string{},
				Arguments: []string{},
			},
			want: []string{},
		},
		{
			name: "flags only",
			input: &clihelper.IacArgs{
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parsed := clihelper.NewIacArgs(tt.input...)
			got := parsed.Slice()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIacArgsAddFlagIfNotPresent(t *testing.T) {
	t.Parallel()

	args := &clihelper.IacArgs{
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

	args := &clihelper.IacArgs{
		Flags: []string{"-auto-approve", "-input=false"},
	}

	assert.True(t, args.HasFlag("-auto-approve"))
	assert.True(t, args.HasFlag("-input"))
	assert.False(t, args.HasFlag("-destroy"))
}

func TestIacArgsRemoveFlag(t *testing.T) {
	t.Parallel()

	args := &clihelper.IacArgs{
		Flags: []string{"-auto-approve", "-input=false", "-destroy"},
	}

	args.RemoveFlag("-input")
	require.Len(t, args.Flags, 2)
	assert.Equal(t, []string{"-auto-approve", "-destroy"}, args.Flags)

	args.RemoveFlag("-auto-approve")
	require.Len(t, args.Flags, 1)
	assert.Equal(t, []string{"-destroy"}, args.Flags)
}

func TestIacArgsAppendArgument(t *testing.T) {
	t.Parallel()

	args := &clihelper.IacArgs{
		Command:   "apply",
		Arguments: []string{"plan1"},
	}

	args.AppendArgument("plan2")
	assert.Equal(t, []string{"plan1", "plan2"}, args.Arguments)
}

func TestIacArgsClone(t *testing.T) {
	t.Parallel()

	original := &clihelper.IacArgs{
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

	args := &clihelper.IacArgs{
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

	args := clihelper.NewIacArgs("apply", "-auto-approve", "tfplan")
	assert.Equal(t, "apply", args.First())

	empty := clihelper.NewEmptyIacArgs()
	assert.Empty(t, empty.First())
}

func TestIacArgsTail(t *testing.T) {
	t.Parallel()

	args := clihelper.NewIacArgs("apply", "-auto-approve", "tfplan")
	assert.Equal(t, []string{"-auto-approve", "tfplan"}, args.Tail())

	empty := clihelper.NewEmptyIacArgs()
	assert.Empty(t, empty.Tail())
}

func TestIacArgsHasPlanFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args     *clihelper.IacArgs
		name     string
		expected bool
	}{
		{
			name:     "empty args",
			args:     clihelper.NewEmptyIacArgs(),
			expected: false,
		},
		{
			name:     "plan with -out flag",
			args:     clihelper.NewIacArgs("plan", "-out=tfplan"),
			expected: true,
		},
		{
			name:     "plan without -out flag",
			args:     clihelper.NewIacArgs("plan", "-input=false"),
			expected: false,
		},
		{
			name:     "apply with plan file argument",
			args:     clihelper.NewIacArgs("apply", "-auto-approve", "tfplan"),
			expected: true,
		},
		{
			name:     "apply without plan file",
			args:     clihelper.NewIacArgs("apply", "-auto-approve"),
			expected: false,
		},
		{
			name:     "destroy with plan file argument",
			args:     clihelper.NewIacArgs("destroy", "tfplan"),
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
		base          *clihelper.IacArgs
		other         *clihelper.IacArgs
		expectedFlags []string
	}{
		{
			name:          "merge into empty",
			base:          clihelper.NewIacArgs("apply"),
			other:         clihelper.NewIacArgs("apply", "-auto-approve", "-input=false"),
			expectedFlags: []string{"-auto-approve", "-input=false"},
		},
		{
			name:          "skip duplicates",
			base:          clihelper.NewIacArgs("apply", "-auto-approve"),
			other:         clihelper.NewIacArgs("apply", "-auto-approve", "-input=false"),
			expectedFlags: []string{"-auto-approve", "-input=false"},
		},
		{
			name:          "merge from empty",
			base:          clihelper.NewIacArgs("apply", "-auto-approve"),
			other:         clihelper.NewEmptyIacArgs(),
			expectedFlags: []string{"-auto-approve"},
		},
		{
			name:          "both have different flags",
			base:          clihelper.NewIacArgs("apply", "-compact-warnings"),
			other:         clihelper.NewIacArgs("apply", "-auto-approve", "-input=false"),
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
