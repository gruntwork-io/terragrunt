package writer_test

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/writer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	syncWriters      = 32
	syncWritesEach   = 256
	syncWritePayload = "the quick brown fox jumps over the lazy dog\n"
)

// TestSyncWriter_Concurrent verifies that a single SyncWriter serializes
// concurrent writes to an unsynchronized bytes.Buffer. Run with -race: the
// detector must stay silent, no byte is lost, and no line is interleaved.
func TestSyncWriter_Concurrent(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	sw := writer.NewSyncWriter(&buf)
	payload := []byte(syncWritePayload)

	var wg sync.WaitGroup

	start := make(chan struct{})

	for range syncWriters {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start

			for range syncWritesEach {
				_, _ = sw.Write(payload)
			}
		}()
	}

	close(start)
	wg.Wait()

	require.Equal(t, len(payload)*syncWriters*syncWritesEach, buf.Len())

	for _, line := range strings.SplitAfter(buf.String(), "\n") {
		if line != "" {
			assert.Equal(t, syncWritePayload, line, "torn write")
		}
	}
}

// TestSyncWriter_Unwrap ensures SyncWriter stays transparent to
// ExtractOriginalWriter so the underlying handle remains reachable.
func TestSyncWriter_Unwrap(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	wrapped := writer.NewOriginalWriter(writer.NewSyncWriter(&buf))

	assert.Same(t, &buf, writer.ExtractOriginalWriter(io.Writer(wrapped)))
}
