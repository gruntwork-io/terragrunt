package dynamodb

import (
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/stretchr/testify/assert"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAcquireLockHappyPath(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	lock := DynamoDbLock{
		StateFileId:    uniqueId(),
		AwsRegion:      DEFAULT_TEST_REGION,
		AwsProfile:     DEFAULT_TEST_PROFILE,
		AwsRoleARN:     DEFAULT_TEST_ROLE_ARN,
		TableName:      uniqueTableNameForTest(),
		MaxLockRetries: 1,
	}

	defer cleanupTableForTest(t, lock.TableName, client)

	err := lock.AcquireLock(mockOptions)
	assert.Nil(t, err)
}

func TestAcquireLockWhenLockIsAlreadyTaken(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	stateFileId := uniqueId()
	lock := DynamoDbLock{
		StateFileId:    stateFileId,
		AwsRegion:      DEFAULT_TEST_REGION,
		AwsProfile:     DEFAULT_TEST_PROFILE,
		AwsRoleARN:     DEFAULT_TEST_ROLE_ARN,
		TableName:      uniqueTableNameForTest(),
		MaxLockRetries: 1,
	}

	defer cleanupTableForTest(t, lock.TableName, client)

	// Acquire the lock the first time
	err := lock.AcquireLock(mockOptions)
	assert.Nil(t, err)

	// Now try to acquire the lock again and make sure you get an error
	err = lock.AcquireLock(mockOptions)
	assert.True(t, errors.IsError(err, AcquireLockRetriesExceeded{ItemId: stateFileId, Retries: 1}), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
}

func TestAcquireAndReleaseLock(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	stateFileId := uniqueId()
	lock := DynamoDbLock{
		StateFileId:    stateFileId,
		AwsRegion:      DEFAULT_TEST_REGION,
		AwsProfile:     DEFAULT_TEST_PROFILE,
		AwsRoleARN:     DEFAULT_TEST_ROLE_ARN,
		TableName:      uniqueTableNameForTest(),
		MaxLockRetries: 1,
	}

	defer cleanupTableForTest(t, lock.TableName, client)

	// Acquire the lock the first time
	err := lock.AcquireLock(mockOptions)
	assert.Nil(t, err)

	// Now try to acquire the lock again and make sure you get an error
	err = lock.AcquireLock(mockOptions)
	assert.True(t, errors.IsError(err, AcquireLockRetriesExceeded{ItemId: stateFileId, Retries: 1}), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)

	// Release the lock
	err = lock.ReleaseLock(mockOptions)
	assert.Nil(t, err)

	// Finally, try to acquire the lock again; you should succeed
	err = lock.AcquireLock(mockOptions)
	assert.Nil(t, err)
}

func TestAcquireLockConcurrency(t *testing.T) {
	t.Parallel()

	concurrency := 20

	withLockTableProvisionedUnits(t, concurrency, concurrency, func(tableName string, client *dynamodb.DynamoDB) {
		stateFileId := uniqueId()
		lock := DynamoDbLock{
			StateFileId:    stateFileId,
			AwsRegion:      DEFAULT_TEST_REGION,
			AwsProfile:     DEFAULT_TEST_PROFILE,
			AwsRoleARN:     DEFAULT_TEST_ROLE_ARN,
			TableName:      uniqueTableNameForTest(),
			MaxLockRetries: 1,
		}

		defer cleanupTableForTest(t, lock.TableName, client)

		// Use a WaitGroup to ensure the test doesn't exit before all goroutines finish.
		var waitGroup sync.WaitGroup
		// This will count how many of the goroutines were able to acquire a lock. We use Go's atomic package to
		// ensure all modifications to this counter are atomic operations.
		locksAcquired := int32(0)

		// Launch a bunch of goroutines who will all try to acquire the lock at more or less the same time.
		// Only one should succeed.
		for i := 0; i < concurrency; i++ {
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				err := lock.AcquireLock(mockOptions)
				if err == nil {
					atomic.AddInt32(&locksAcquired, 1)
				} else {
					assert.True(t, errors.IsError(err, AcquireLockRetriesExceeded{ItemId: stateFileId, Retries: 1}), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
				}
			}()
		}

		waitGroup.Wait()

		assert.Equal(t, int32(1), locksAcquired, "Only one of the goroutines should have been able to acquire a lock")
	})
}
