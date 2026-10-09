package backend_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/stretchr/testify/assert"
)

// TestCommonBackendBootstrapsEachBucketOnceWithRacing pins that parallel callers sharing a
// bucket run its bootstrap once. Each caller follows the remote state backends: it locks the
// bucket mutex, returns if the config is inited, and otherwise bootstraps and marks it.
// Two buckets run side by side, so init marks for different buckets are written concurrently.
func TestCommonBackendBootstrapsEachBucketOnceWithRacing(t *testing.T) {
	t.Parallel()

	const callersPerBucket = 8

	var (
		start   = make(chan struct{})
		group   sync.WaitGroup
		buckets = []string{"bucket-a", "bucket-b"}
	)

	common := backend.NewCommonBackend("test")
	bootstraps := make([]atomic.Int32, len(buckets))
	mutexes := make([][]*sync.Mutex, len(buckets))

	for b, bucket := range buckets {
		mutexes[b] = make([]*sync.Mutex, callersPerBucket)

		for i := range callersPerBucket {
			group.Go(func() {
				<-start

				mu := common.GetBucketMutex(bucket)
				mutexes[b][i] = mu

				mu.Lock()
				defer mu.Unlock()

				cfg := bucketCacheKey(bucket)
				if common.IsConfigInited(cfg) {
					return
				}

				bootstraps[b].Add(1)
				common.MarkConfigInited(cfg)
			})
		}
	}

	close(start)
	group.Wait()

	for b, bucket := range buckets {
		assert.Equalf(t, int32(1), bootstraps[b].Load(), "bucket %s", bucket)

		for i, mu := range mutexes[b] {
			assert.Samef(t, mutexes[b][0], mu, "bucket %s caller %d", bucket, i)
		}
	}

	assert.NotSame(t, mutexes[0][0], mutexes[1][0], "each bucket must get its own mutex")
}

type bucketCacheKey string

func (key bucketCacheKey) CacheKey() string {
	return string(key)
}
