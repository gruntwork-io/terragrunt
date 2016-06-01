package dynamodb

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"sync/atomic"
	"sync"
)

func TestAcquireLockHappyPath(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	lock := DynamoLock{
		StateFileId: uniqueId(),
		AwsRegion: DEFAULT_TEST_REGION,
		TableName: uniqueTableNameForTest(),
		MaxLockRetries: 1,
	}

	defer cleanupTable(t, lock.TableName, client)

	err := lock.AcquireLock()
	assert.Nil(t, err)
}

func TestAcquireLockWhenLockIsAlreadyTaken(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	lock := DynamoLock{
		StateFileId: uniqueId(),
		AwsRegion: DEFAULT_TEST_REGION,
		TableName: uniqueTableNameForTest(),
		MaxLockRetries: 1,
	}

	defer cleanupTable(t, lock.TableName, client)

	// Acquire the lock the first time
	err := lock.AcquireLock()
	assert.Nil(t, err)

	// Now try to acquire the lock again and make sure you get an error
	err = lock.AcquireLock()
	assert.NotNil(t, err)
}

func TestAcquireAndReleaseLock(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	lock := DynamoLock{
		StateFileId: uniqueId(),
		AwsRegion: DEFAULT_TEST_REGION,
		TableName: uniqueTableNameForTest(),
		MaxLockRetries: 1,
	}

	defer cleanupTable(t, lock.TableName, client)

	// Acquire the lock the first time
	err := lock.AcquireLock()
	assert.Nil(t, err)

	// Now try to acquire the lock again and make sure you get an error
	err = lock.AcquireLock()
	assert.NotNil(t, err)

	// Release the lock
	err = lock.ReleaseLock()
	assert.Nil(t, err)

	// Finally, try to acquire the lock again; you should succeed
	err = lock.AcquireLock()
	assert.Nil(t, err)
}

func TestAcquireLockConcurrency(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	lock := DynamoLock{
		StateFileId: uniqueId(),
		AwsRegion: DEFAULT_TEST_REGION,
		TableName: uniqueTableNameForTest(),
		MaxLockRetries: 1,
	}

	defer cleanupTable(t, lock.TableName, client)

	// Use a WaitGroup to ensure the test doesn't exit before all goroutines finish.
	var waitGroup sync.WaitGroup
	// This will count how many of the goroutines were able to acquire a lock. We use Go's atomic package to
	// ensure all modifications to this counter are atomic operations.
	locksAcquired := int32(0)

	// Launch a bunch of goroutines who will all try to acquire the lock at more or less the same time.
	// Only one should succeed.
	for i := 0; i < 20; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			err := lock.AcquireLock()
			if err == nil {
				atomic.AddInt32(&locksAcquired, 1)
			}
		}()
	}

	waitGroup.Wait()

	assert.Equal(t, int32(1), locksAcquired, "Only one of the goroutines should have been able to acquire a lock")
}