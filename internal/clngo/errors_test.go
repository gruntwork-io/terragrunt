package clngo_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clngo"
	"github.com/stretchr/testify/assert"
)

func TestErrorString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  clngo.Error
		want string
	}{
		{
			name: "temp dir error",
			err:  clngo.ErrTempDir,
			want: "failed to create or manage temporary directory",
		},
		{
			name: "git clone error",
			err:  clngo.ErrGitClone,
			want: "failed to complete git clone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}

func TestWrappedError(t *testing.T) {
	t.Parallel()
	baseErr := errors.New("base error")
	tests := []struct {
		name    string
		wrapped *clngo.WrappedError
		want    string
	}{
		{
			name: "with path",
			wrapped: &clngo.WrappedError{
				Op:   "clone",
				Path: "/tmp/repo",
				Err:  baseErr,
			},
			want: "clone: /tmp/repo: base error",
		},
		{
			name: "with context",
			wrapped: &clngo.WrappedError{
				Op:      "clone",
				Context: "repository not found",
				Err:     baseErr,
			},
			want: "clone: repository not found: base error",
		},
		{
			name: "basic",
			wrapped: &clngo.WrappedError{
				Op:  "clone",
				Err: baseErr,
			},
			want: "clone: base error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.wrapped.Error())
			assert.Equal(t, baseErr, tt.wrapped.Unwrap())
		})
	}
}
