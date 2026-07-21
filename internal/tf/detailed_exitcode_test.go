package tf_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/stretchr/testify/assert"
)

func TestDetailedExitCodeMapFinal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		codes           map[string]int
		name            string
		wantNonDetailed int
		wantDetailed    int
	}{
		{
			name:            "empty map returns success in both modes",
			codes:           nil,
			wantNonDetailed: tf.DetailedExitCodeSuccess,
			wantDetailed:    tf.DetailedExitCodeSuccess,
		},
		{
			name: "all success",
			codes: map[string]int{
				"a": tf.DetailedExitCodeSuccess,
				"b": tf.DetailedExitCodeSuccess,
			},
			wantNonDetailed: tf.DetailedExitCodeSuccess,
			wantDetailed:    tf.DetailedExitCodeSuccess,
		},
		{
			name: "only changes returns 2 in both modes",
			codes: map[string]int{
				"a": tf.DetailedExitCodeChanges,
				"b": tf.DetailedExitCodeSuccess,
			},
			wantNonDetailed: tf.DetailedExitCodeChanges,
			wantDetailed:    tf.DetailedExitCodeChanges,
		},
		{
			name: "changes plus error - dispatch is observable (max=2 vs error precedence=1)",
			codes: map[string]int{
				"a": tf.DetailedExitCodeChanges,
				"b": tf.DetailedExitCodeError,
			},
			wantNonDetailed: tf.DetailedExitCodeChanges,
			wantDetailed:    tf.DetailedExitCodeError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			em := tf.NewDetailedExitCodeMap()
			for path, code := range tc.codes {
				em.Set(path, code)
			}

			assert.Equal(t, tc.wantNonDetailed, em.Final(false), "non-detailed mode")
			assert.Equal(t, tc.wantDetailed, em.Final(true), "detailed mode")
		})
	}
}
