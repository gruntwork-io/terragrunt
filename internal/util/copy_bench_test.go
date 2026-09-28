package util_test

import (
	"bytes"
	"io"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blackholeWriter keeps the destination out of the timings, leaving the copy
// machinery as the only thing they measure.
type blackholeWriter struct{}

func (blackholeWriter) Write(p []byte) (int, error) { return len(p), nil }

// BenchmarkCopy moves a payload through the context-cancellable copy. The sizes are
// the three things the provider cache puts through it: a detached signature, a
// SHA256SUMS document, and a provider archive. The scratch space is claimed once per
// call, so it is most of the cost of the first two and invisible against the third.
func BenchmarkCopy(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{name: "signature", size: 566},
		{name: "checksums", size: 1600},
		{name: "archive", size: 64 << 20},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			payload := bytes.Repeat([]byte("terragrunt"), size.size/10+1)[:size.size]
			ctx := b.Context()
			dst := blackholeWriter{}

			b.ReportAllocs()
			b.SetBytes(int64(size.size))

			for b.Loop() {
				_, err := util.Copy(ctx, dst, bytes.NewReader(payload))
				require.NoError(b, err)
			}
		})
	}
}

// BenchmarkCopyConcurrent runs the copies in parallel, the way the provider cache
// reaches this code when it warms several providers at once. A pool holding a single
// buffer shows up here as contention.
func BenchmarkCopyConcurrent(b *testing.B) {
	for _, workers := range []int{2, 8} {
		b.Run(strconv.Itoa(workers)+"workers", func(b *testing.B) {
			const payloadSize = 64 << 10

			payload := bytes.Repeat([]byte("terragrunt"), payloadSize/10+1)[:payloadSize]
			ctx := b.Context()

			b.SetParallelism(workers)
			b.ReportAllocs()
			b.SetBytes(payloadSize)

			b.RunParallel(func(pb *testing.PB) {
				var dst io.Writer = blackholeWriter{}

				for pb.Next() {
					_, err := util.Copy(ctx, dst, bytes.NewReader(payload))

					// require.NoError here would call Goexit off the benchmark
					// goroutine, so the failure is recorded instead.
					if !assert.NoError(b, err) {
						return
					}
				}
			})
		})
	}
}
