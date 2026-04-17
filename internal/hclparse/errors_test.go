package hclparse_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/stretchr/testify/assert"
)

func TestTypedErrors_Messages(t *testing.T) {
	t.Parallel()

	baseErr := errors.New("base error")

	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{name: "UnexpectedBodyTypeError", err: hclparse.UnexpectedBodyTypeError{FilePath: "test.hcl"}, contains: "test.hcl"},
		{name: "DuplicateUnitNameError", err: hclparse.DuplicateUnitNameError{Name: "vpc"}, contains: "vpc"},
		{name: "DuplicateStackNameError", err: hclparse.DuplicateStackNameError{Name: "infra"}, contains: "infra"},
		{name: "IncludeValidationError", err: hclparse.IncludeValidationError{IncludeName: "shared", Reason: "has locals"}, contains: "shared"},
		{name: "FileReadError", err: hclparse.FileReadError{FilePath: "missing.hcl", Err: baseErr}, contains: "missing.hcl"},
		{name: "FileParseError", err: hclparse.FileParseError{FilePath: "bad.hcl", Detail: "syntax"}, contains: "bad.hcl"},
		{name: "FileDecodeError", err: hclparse.FileDecodeError{Name: "inc", Detail: "decode failed"}, contains: "inc"},
		{name: "FileWriteError", err: hclparse.FileWriteError{FilePath: "out.hcl", Err: baseErr}, contains: "out.hcl"},
		{name: "DirCreateError", err: hclparse.DirCreateError{DirPath: "/tmp/dir", Err: baseErr}, contains: "/tmp/dir"},
		{name: "LocalEvalError", err: hclparse.LocalEvalError{Name: "env", Detail: "unknown var"}, contains: "env"},
		{name: "LocalsCycleError", err: hclparse.LocalsCycleError{Names: []string{"a", "b"}}, contains: "[a b]"},
		{name: "LocalsMaxIterError", err: hclparse.LocalsMaxIterError{MaxIterations: 100, Remaining: 5}, contains: "100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			msg := tt.err.Error()
			assert.Contains(t, msg, tt.contains)
			assert.NotEmpty(t, msg)
		})
	}
}

func TestTypedErrors_Unwrap(t *testing.T) {
	t.Parallel()

	baseErr := errors.New("root cause")

	t.Run("FileReadError", func(t *testing.T) {
		t.Parallel()

		err := hclparse.FileReadError{FilePath: "f.hcl", Err: baseErr}
		assert.ErrorIs(t, err, baseErr)
	})

	t.Run("FileWriteError", func(t *testing.T) {
		t.Parallel()

		err := hclparse.FileWriteError{FilePath: "f.hcl", Err: baseErr}
		assert.ErrorIs(t, err, baseErr)
	})

	t.Run("DirCreateError", func(t *testing.T) {
		t.Parallel()

		err := hclparse.DirCreateError{DirPath: "/d", Err: baseErr}
		assert.ErrorIs(t, err, baseErr)
	})
}
