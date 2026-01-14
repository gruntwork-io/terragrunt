package tofucmd_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tofucmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cmd      *tofucmd.TofuCommand
		expected []string
	}{
		{
			name:     "nil receiver returns nil",
			cmd:      nil,
			expected: nil,
		},
		{
			name: "command only, no args",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{},
			},
			expected: []string{"plan"},
		},
		{
			name: "command with single arg",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-auto-approve"},
			},
			expected: []string{"apply", "-auto-approve"},
		},
		{
			name: "command with multiple args",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{"-out=plan.tfplan", "-var", "foo=bar", "-no-color"},
			},
			expected: []string{"plan", "-out=plan.tfplan", "-var", "foo=bar", "-no-color"},
		},
		{
			name: "destroy command",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "destroy",
				Args: []string{"-auto-approve", "-target=module.foo"},
			},
			expected: []string{"destroy", "-auto-approve", "-target=module.foo"},
		},
		{
			name: "init command with backend config",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "init",
				Args: []string{"-backend-config=path/to/backend.hcl", "-reconfigure"},
			},
			expected: []string{"init", "-backend-config=path/to/backend.hcl", "-reconfigure"},
		},
		{
			name: "empty command string",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "",
				Args: []string{"arg1"},
			},
			expected: []string{"", "arg1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.cmd.ProcessArgs()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFirstArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cmd      *tofucmd.TofuCommand
		expected string
	}{
		{
			name:     "nil receiver returns empty string",
			cmd:      nil,
			expected: "",
		},
		{
			name: "empty args returns empty string",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{},
			},
			expected: "",
		},
		{
			name: "nil args returns empty string",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: nil,
			},
			expected: "",
		},
		{
			name: "single arg returns that arg",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-auto-approve"},
			},
			expected: "-auto-approve",
		},
		{
			name: "multiple args returns first",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{"-out=plan.tfplan", "-var", "foo=bar"},
			},
			expected: "-out=plan.tfplan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.cmd.FirstArg()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLastArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cmd      *tofucmd.TofuCommand
		expected string
	}{
		{
			name:     "nil receiver returns empty string",
			cmd:      nil,
			expected: "",
		},
		{
			name: "empty args returns empty string",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{},
			},
			expected: "",
		},
		{
			name: "nil args returns empty string",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: nil,
			},
			expected: "",
		},
		{
			name: "single arg returns that arg",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-auto-approve"},
			},
			expected: "-auto-approve",
		},
		{
			name: "multiple args returns last",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{"-out=plan.tfplan", "-var", "foo=bar"},
			},
			expected: "foo=bar",
		},
		{
			name: "plan file as last arg",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-auto-approve", "plan.tfplan"},
			},
			expected: "plan.tfplan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.cmd.LastArg()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cmd      *tofucmd.TofuCommand
		arg      string
		expected bool
	}{
		{
			name:     "nil receiver returns false",
			cmd:      nil,
			arg:      "-auto-approve",
			expected: false,
		},
		{
			name: "empty args returns false",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{},
			},
			arg:      "-out=plan.tfplan",
			expected: false,
		},
		{
			name: "arg exists returns true",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-auto-approve", "-no-color"},
			},
			arg:      "-auto-approve",
			expected: true,
		},
		{
			name: "arg does not exist returns false",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-auto-approve", "-no-color"},
			},
			arg:      "-input=false",
			expected: false,
		},
		{
			name: "arg at end exists returns true",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{"-out=plan.tfplan", "-var", "foo=bar"},
			},
			arg:      "foo=bar",
			expected: true,
		},
		{
			name: "partial match returns false",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{"-out=plan.tfplan"},
			},
			arg:      "-out",
			expected: false,
		},
		{
			name: "exact match required",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{"-auto-approve"},
			},
			arg:      "-auto-approv",
			expected: false,
		},
		{
			name: "empty string arg check",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{"", "-auto-approve"},
			},
			arg:      "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.cmd.HasArg(tt.arg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInsertArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cmd          *tofucmd.TofuCommand
		name         string
		arg          string
		expectedArgs []string
		position     int
	}{
		{
			name:         "nil receiver does nothing",
			cmd:          nil,
			arg:          "-auto-approve",
			position:     0,
			expectedArgs: nil,
		},
		{
			name: "insert at beginning of empty args",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{},
			},
			arg:          "-auto-approve",
			position:     0,
			expectedArgs: []string{"-auto-approve"},
		},
		{
			name: "insert at beginning",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-no-color"},
			},
			arg:          "-auto-approve",
			position:     0,
			expectedArgs: []string{"-auto-approve", "-no-color"},
		},
		{
			name: "insert in middle",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{"-out=plan.tfplan", "-no-color"},
			},
			arg:          "-input=false",
			position:     1,
			expectedArgs: []string{"-out=plan.tfplan", "-input=false", "-no-color"},
		},
		{
			name: "insert at end",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{"-out=plan.tfplan"},
			},
			arg:          "-no-color",
			position:     1,
			expectedArgs: []string{"-out=plan.tfplan", "-no-color"},
		},
		{
			name: "skip if arg already exists",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-auto-approve", "-no-color"},
			},
			arg:          "-auto-approve",
			position:     1,
			expectedArgs: []string{"-auto-approve", "-no-color"},
		},
		{
			name: "skip if arg exists at different position",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-no-color", "-auto-approve"},
			},
			arg:          "-auto-approve",
			position:     0,
			expectedArgs: []string{"-no-color", "-auto-approve"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Clone to avoid modifying the original in parallel tests
			var cmd *tofucmd.TofuCommand
			if tt.cmd != nil {
				cmd = &tofucmd.TofuCommand{
					Cmd:  tt.cmd.Cmd,
					Args: append([]string{}, tt.cmd.Args...),
				}
			}

			if cmd != nil {
				cmd.InsertArg(tt.arg, tt.position)
				assert.Equal(t, tt.expectedArgs, cmd.Args)
			} else {
				// Verify nil receiver doesn't panic
				cmd.InsertArg(tt.arg, tt.position)
			}
		})
	}
}

func TestAppendArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		cmd          *tofucmd.TofuCommand
		arg          string
		expectedArgs []string
	}{
		{
			name:         "nil receiver does nothing",
			cmd:          nil,
			arg:          "-auto-approve",
			expectedArgs: nil,
		},
		{
			name: "append to empty args",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{},
			},
			arg:          "-auto-approve",
			expectedArgs: []string{"-auto-approve"},
		},
		{
			name: "append to existing args",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-auto-approve"},
			},
			arg:          "-no-color",
			expectedArgs: []string{"-auto-approve", "-no-color"},
		},
		{
			name: "append allows duplicates",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-auto-approve"},
			},
			arg:          "-auto-approve",
			expectedArgs: []string{"-auto-approve", "-auto-approve"},
		},
		{
			name: "append empty string",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "plan",
				Args: []string{"-out=plan.tfplan"},
			},
			arg:          "",
			expectedArgs: []string{"-out=plan.tfplan", ""},
		},
		{
			name: "append plan file path",
			cmd: &tofucmd.TofuCommand{
				Cmd:  "apply",
				Args: []string{"-auto-approve"},
			},
			arg:          "/path/to/plan.tfplan",
			expectedArgs: []string{"-auto-approve", "/path/to/plan.tfplan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Clone to avoid modifying the original in parallel tests
			var cmd *tofucmd.TofuCommand
			if tt.cmd != nil {
				cmd = &tofucmd.TofuCommand{
					Cmd:  tt.cmd.Cmd,
					Args: append([]string{}, tt.cmd.Args...),
				}
			}

			if cmd != nil {
				cmd.AppendArg(tt.arg)
				assert.Equal(t, tt.expectedArgs, cmd.Args)
			} else {
				// Verify nil receiver doesn't panic
				cmd.AppendArg(tt.arg)
			}
		})
	}
}

func TestClone(t *testing.T) {
	t.Parallel()

	t.Run("nil receiver returns nil", func(t *testing.T) {
		t.Parallel()

		var cmd *tofucmd.TofuCommand

		result := cmd.Clone()
		assert.Nil(t, result)
	})

	t.Run("clone creates independent copy", func(t *testing.T) {
		t.Parallel()

		original := &tofucmd.TofuCommand{
			Cmd:  "plan",
			Args: []string{"-out=plan.tfplan", "-no-color"},
		}

		cloned := original.Clone()

		// Verify values are equal
		require.NotNil(t, cloned)
		assert.Equal(t, original.Cmd, cloned.Cmd)
		assert.Equal(t, original.Args, cloned.Args)

		// Verify they are independent (modifying one doesn't affect the other)
		cloned.Cmd = "apply"
		cloned.Args[0] = "-auto-approve"

		assert.Equal(t, "plan", original.Cmd)
		assert.Equal(t, "-out=plan.tfplan", original.Args[0])
	})

	t.Run("clone with empty args", func(t *testing.T) {
		t.Parallel()

		original := &tofucmd.TofuCommand{
			Cmd:  "init",
			Args: []string{},
		}

		cloned := original.Clone()

		require.NotNil(t, cloned)
		assert.Equal(t, "init", cloned.Cmd)
		assert.Empty(t, cloned.Args)
	})

	t.Run("clone with nil args", func(t *testing.T) {
		t.Parallel()

		original := &tofucmd.TofuCommand{
			Cmd:  "version",
			Args: nil,
		}

		cloned := original.Clone()

		require.NotNil(t, cloned)
		assert.Equal(t, "version", cloned.Cmd)
		assert.Nil(t, cloned.Args)
	})

	t.Run("modifying original does not affect clone", func(t *testing.T) {
		t.Parallel()

		original := &tofucmd.TofuCommand{
			Cmd:  "destroy",
			Args: []string{"-auto-approve"},
		}

		cloned := original.Clone()

		// Modify original
		original.Args = append(original.Args, "-target=module.foo")

		// Clone should not be affected
		assert.Equal(t, []string{"-auto-approve"}, cloned.Args)
		assert.Len(t, cloned.Args, 1)
	})
}

func TestMultipleOperations(t *testing.T) {
	t.Parallel()

	t.Run("build command through multiple operations", func(t *testing.T) {
		t.Parallel()

		cmd := &tofucmd.TofuCommand{
			Cmd:  "apply",
			Args: []string{},
		}

		// Build up the command
		cmd.InsertArg("-input=false", 0)
		cmd.AppendArg("-auto-approve")
		cmd.AppendArg("-no-color")

		assert.Equal(t, []string{"-input=false", "-auto-approve", "-no-color"}, cmd.Args)
		assert.Equal(t, []string{"apply", "-input=false", "-auto-approve", "-no-color"}, cmd.ProcessArgs())
		assert.True(t, cmd.HasArg("-auto-approve"))
		assert.Equal(t, "-input=false", cmd.FirstArg())
		assert.Equal(t, "-no-color", cmd.LastArg())
	})

	t.Run("clone and modify independently", func(t *testing.T) {
		t.Parallel()

		original := &tofucmd.TofuCommand{
			Cmd:  "plan",
			Args: []string{"-out=plan.tfplan"},
		}

		cloned := original.Clone()
		cloned.AppendArg("-no-color")
		cloned.InsertArg("-input=false", 0)

		// Original should be unchanged
		assert.Equal(t, []string{"-out=plan.tfplan"}, original.Args)
		// Cloned should have modifications
		assert.Equal(t, []string{"-input=false", "-out=plan.tfplan", "-no-color"}, cloned.Args)
	})

	t.Run("insert prevents duplicates but append allows them", func(t *testing.T) {
		t.Parallel()

		cmd := &tofucmd.TofuCommand{
			Cmd:  "apply",
			Args: []string{},
		}

		cmd.InsertArg("-auto-approve", 0)
		cmd.InsertArg("-auto-approve", 0) // Should be skipped
		cmd.AppendArg("-auto-approve")    // Should be added

		assert.Equal(t, []string{"-auto-approve", "-auto-approve"}, cmd.Args)
	})
}

func TestEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("ProcessArgs does not share underlying slice", func(t *testing.T) {
		t.Parallel()

		cmd := &tofucmd.TofuCommand{
			Cmd:  "plan",
			Args: []string{"-no-color"},
		}

		result := cmd.ProcessArgs()
		result[0] = "modified"

		// Original should not be affected
		assert.Equal(t, "plan", cmd.Cmd)
	})

	t.Run("special characters in args", func(t *testing.T) {
		t.Parallel()

		cmd := &tofucmd.TofuCommand{
			Cmd:  "apply",
			Args: []string{"-var", "json={\"key\":\"value\"}", "-var", "list=[\"a\",\"b\"]"},
		}

		assert.True(t, cmd.HasArg("json={\"key\":\"value\"}"))
		assert.Equal(t, "list=[\"a\",\"b\"]", cmd.LastArg())
	})

	t.Run("very long args list", func(t *testing.T) {
		t.Parallel()

		args := make([]string, 1000)
		for i := range args {
			args[i] = "-var"
		}

		cmd := &tofucmd.TofuCommand{
			Cmd:  "plan",
			Args: args,
		}

		assert.Equal(t, "-var", cmd.FirstArg())
		assert.Equal(t, "-var", cmd.LastArg())
		assert.Len(t, cmd.ProcessArgs(), 1001)
	})
}
