package util_test

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// TestCopyConcurrentWithRacing runs copies of different payloads at the same time.
// They share the scratch space the copy reads through, so a buffer handed to two
// callers at once shows up as one copy's content landing in another's output.
func TestCopyConcurrentWithRacing(t *testing.T) {
	t.Parallel()

	const (
		workers     = 8
		payloadSize = 128 << 10
	)

	payloads := make([][]byte, workers)
	for i := range payloads {
		payloads[i] = bytes.Repeat([]byte(strconv.Itoa(i)), payloadSize)
	}

	results := make([]*bytes.Buffer, workers)

	group, ctx := errgroup.WithContext(t.Context())

	for i := range workers {
		results[i] = &bytes.Buffer{}

		group.Go(func() error {
			_, err := util.Copy(ctx, results[i], bytes.NewReader(payloads[i]))

			return err
		})
	}

	require.NoError(t, group.Wait())

	for i := range workers {
		assert.True(
			t,
			bytes.Equal(payloads[i], results[i].Bytes()),
			"copy %d received bytes from another copy",
			i,
		)
	}
}
