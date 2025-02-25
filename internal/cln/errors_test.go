package cln_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cln"
	"github.com/stretchr/testify/assert"
)

func TestErrorString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  cln.Error
		want string
	}{
		{
			name: "temp dir error",
			err:  cln.ErrTempDir,
			want: "failed to create or manage temporary directory",
		},
		{
			name: "git clone error",
			err:  cln.ErrGitClone,
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
		wrapped *cln.WrappedError
		want    string
	}{
		{
			name: "with path",
			wrapped: &cln.WrappedError{
				Op:   "clone",
				Path: "/tmp/repo",
				Err:  baseErr,
			},
			want: "clone: /tmp/repo: base error",
		},
		{
			name: "with context",
			wrapped: &cln.WrappedError{
				Op:      "clone",
				Context: "repository not found",
				Err:     baseErr,
			},
			want: "clone: repository not found: base error",
		},
		{
			name: "basic",
			wrapped: &cln.WrappedError{
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
