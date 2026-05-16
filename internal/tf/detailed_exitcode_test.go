package tf_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/stretchr/testify/assert"
)

func TestDetailedExitCodeMapFinal(t *testing.T) {
	t.Parallel()

	// Final must dispatch to GetFinalExitCode vs GetFinalDetailedExitCode
	// based on the boolean. With a "changes" + "error" combination the
	// two strategies disagree (max-wins vs error-precedence) so this is
	// a clean way to confirm the dispatch.
	em := tf.NewDetailedExitCodeMap()
	em.Set("a", tf.DetailedExitCodeChanges) // 2
	em.Set("b", tf.DetailedExitCodeError)   // 1

	assert.Equal(t, tf.DetailedExitCodeChanges, em.Final(false), "non-detailed mode returns the max code (2 wins over 1)")
	assert.Equal(t, tf.DetailedExitCodeError, em.Final(true), "detailed mode gives error precedence over changes")
}
