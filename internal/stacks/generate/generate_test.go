// White-box test file: needs access to the unexported generateLocks.
//
//nolint:testpackage // white-box testing of package-level mutex
package generate

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGenerateLocksSerializeSameKeyWithRacing asserts that two goroutines
// locking the same key on the package-level generateLocks execute their
// critical sections serially, never in parallel. The WithRacing suffix
// routes this test into the `-race` matrix; under that harness a
// hypothetical race on the shared KeyLocks state would be flagged by the
// race detector.
//
// Two goroutines is the smallest size that can demonstrate mutual
// exclusion. Correctness here combined with -race coverage is a stronger
// signal than running N goroutines without -race.
func TestGenerateLocksSerializeSameKeyWithRacing(t *testing.T) {
	t.Parallel()

	const key = "/tmp/test-key-serialize"

	var active atomic.Int32

	runCritical := func() {
		a := active.Add(1)
		require.Equal(t, int32(1), a, "mutex allowed a second concurrent holder of the same key")
		active.Add(-1)
	}

	done := make(chan struct{}, 2)

	for range 2 {
		go func() {
			generateLocks.Lock(key)
			runCritical()
			generateLocks.Unlock(key)

			done <- struct{}{}
		}()
	}

	<-done
	<-done
}
