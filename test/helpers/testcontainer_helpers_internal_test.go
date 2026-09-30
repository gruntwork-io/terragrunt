package helpers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTLoggerPrintf(t *testing.T) {
	t.Parallel()

	logged := &thTRecordingTB{}

	tLogger{tb: logged}.Printf("pulled %s in %d ms", "alpine", 12)

	assert.Equal(t, "pulled alpine in 12 ms", logged.message)
}

// thTRecordingTB is a testing.TB that records the last message logged
// through it. Only Helper and Logf are implemented; tLogger calls nothing else.
type thTRecordingTB struct {
	testing.TB

	message string
}

func (tb *thTRecordingTB) Helper() {}

func (tb *thTRecordingTB) Logf(format string, args ...any) {
	tb.message = fmt.Sprintf(format, args...)
}
