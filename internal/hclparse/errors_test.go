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
		{"UnexpectedBodyTypeError", hclparse.UnexpectedBodyTypeError{FilePath: "test.hcl"}, "test.hcl"},
		{"DuplicateUnitNameError", hclparse.DuplicateUnitNameError{Name: "vpc"}, "vpc"},
		{"DuplicateStackNameError", hclparse.DuplicateStackNameError{Name: "infra"}, "infra"},
		{"IncludeValidationError", hclparse.IncludeValidationError{IncludeName: "shared", Reason: "has locals"}, "shared"},
		{"FileReadError", hclparse.FileReadError{FilePath: "missing.hcl", Err: baseErr}, "missing.hcl"},
		{"FileParseError", hclparse.FileParseError{FilePath: "bad.hcl", Detail: "syntax"}, "bad.hcl"},
		{"FileDecodeError", hclparse.FileDecodeError{Name: "inc", Detail: "decode failed"}, "inc"},
		{"FileWriteError", hclparse.FileWriteError{FilePath: "out.hcl", Err: baseErr}, "out.hcl"},
		{"DirCreateError", hclparse.DirCreateError{DirPath: "/tmp/dir", Err: baseErr}, "/tmp/dir"},
		{"LocalEvalError", hclparse.LocalEvalError{Name: "env", Detail: "unknown var"}, "env"},
		{"LocalsCycleError", hclparse.LocalsCycleError{Names: []string{"a", "b"}}, "[a b]"},
		{"LocalsMaxIterError", hclparse.LocalsMaxIterError{MaxIterations: 100, Remaining: 5}, "100"},
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
