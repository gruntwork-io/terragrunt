package util_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
)

func TestIsCommandExecutable(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		result vexec.Result
		want   bool
	}{
		{
			name:   "handler returns clean exit",
			result: vexec.Result{},
			want:   true,
		},
		{
			name:   "handler returns non-zero exit",
			result: vexec.Result{ExitCode: 1},
			want:   false,
		},
		{
			name:   "handler returns spawn error",
			result: vexec.Result{Err: errors.New("fork/exec: no such file")},
			want:   false,
		},
		{
			name:   "handler writes output but exits clean",
			result: vexec.Result{Stdout: []byte("hello"), Stderr: []byte("warn")},
			want:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
				return tc.result
			})

			got := util.IsCommandExecutable(e, t.Context(), "whatever", "-version")
			assert.Equal(t, tc.want, got)
		})
	}
}
