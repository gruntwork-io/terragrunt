package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoIncludeParserStageError(t *testing.T) {
	t.Parallel()

	cause := errors.New("boom")
	err := AutoIncludeParserStageError{Stage: "rescope", File: "/abs/dir/terragrunt.stack.hcl", Err: cause}

	t.Run("Error message contains stage, file, and cause", func(t *testing.T) {
		t.Parallel()

		msg := err.Error()
		assert.Contains(t, msg, "rescope")
		assert.Contains(t, msg, "/abs/dir/terragrunt.stack.hcl")
		assert.Contains(t, msg, "boom")
	})

	t.Run("Unwrap returns the underlying cause", func(t *testing.T) {
		t.Parallel()
		assert.Same(t, cause, err.Unwrap())
	})

	t.Run("errors.Is sees the underlying sentinel", func(t *testing.T) {
		t.Parallel()
		assert.ErrorIs(t, err, cause)
	})

	t.Run("errors.As extracts the typed wrapper", func(t *testing.T) {
		t.Parallel()

		var extracted AutoIncludeParserStageError
		require.ErrorAs(t, error(err), &extracted)
		assert.Equal(t, "rescope", extracted.Stage)
		assert.Equal(t, "/abs/dir/terragrunt.stack.hcl", extracted.File)
	})

	t.Run("Stage values are distinguishable", func(t *testing.T) {
		t.Parallel()

		stages := []string{"rescope", "eval-context", "parse"}
		for _, stage := range stages {
			e := AutoIncludeParserStageError{Stage: stage, File: "f", Err: cause}
			assert.Contains(t, e.Error(), stage)
		}
	})
}
