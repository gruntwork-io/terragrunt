package config_test

import (
	"strconv"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilesReadDeduplicates(t *testing.T) {
	t.Parallel()

	f := config.NewFilesRead()
	f.Add("a")
	f.Add("b")
	f.Add("a")

	assert.Equal(t, []string{"a", "b"}, f.Paths())
	assert.Equal(t, 2, f.Len())
}

func TestFilesReadNilReceiver(t *testing.T) {
	t.Parallel()

	var f *config.FilesRead

	assert.NotPanics(t, func() { f.Add("a") })
	assert.Nil(t, f.Paths())
	assert.Equal(t, 0, f.Len())
}

// TestFilesReadConcurrentAddWithRacing pins the concurrency-safety of FilesRead.
// Without the mutex, concurrent Add calls produce a slice header / backing
// array mismatch that crashes inside slices.Contains (the original panic seen
// in TestStackOutputsRaw). The -race suffix flags this test for CI execution
// under `go test -race`.
func TestFilesReadConcurrentAddWithRacing(t *testing.T) {
	t.Parallel()

	const (
		workers       = 32
		addsPerWorker = 200
	)

	f := config.NewFilesRead()

	var wg sync.WaitGroup
	wg.Add(workers)

	for w := range workers {
		go func() {
			defer wg.Done()

			for i := range addsPerWorker {
				f.Add(strconv.Itoa(w*addsPerWorker + i))
				_ = f.Paths()
			}
		}()
	}

	wg.Wait()

	require.Equal(t, workers*addsPerWorker, f.Len())
}
