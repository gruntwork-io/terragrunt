package dynamodb

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"time"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"sync"
	"sync/atomic"
	"reflect"
)

func TestCreateLockTableIfNecessaryTableDoesntAlreadyExist(t *testing.T) {
	t.Parallel()

	withLockTable(t, func(tableName string, client *dynamodb.DynamoDB) {
		assertCanWriteToTable(t, tableName, client)
	})
}

func TestCreateLockTableConcurrency(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	tableName := uniqueTableNameForTest()

	defer cleanupTable(t, tableName, client)

	// Use a WaitGroup to ensure the test doesn't exit before all goroutines finish.
	var waitGroup sync.WaitGroup

	// Launch a bunch of goroutines who will all try to create the same table at more or less the same time.
	// DynamoDB will, of course, only allow a single table to be created, but we still need to make sure none of
	// the goroutines report an error.
	for i := 0; i < 20; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			err := createLockTableIfNecessary(tableName, client)
			assert.Nil(t, err)
		}()
	}

	waitGroup.Wait()
}

func TestWaitForTableToBeActiveTableDoesNotExist(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	tableName := "table-does-not-exist"
	retries := 5

	err := waitForTableToBeActive(tableName, client, retries, 1 * time.Millisecond)

	assert.True(t, errors.IsError(err, TableActiveRetriesExceeded{TableName: tableName, Retries: retries}), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
}

func TestCreateLockTableIfNecessaryTableAlreadyExists(t *testing.T) {
	t.Parallel()

	// Create the table the first time
	withLockTable(t, func(tableName string, client *dynamodb.DynamoDB) {
		assertCanWriteToTable(t, tableName, client)

		// Try to create the table the second time and make sure you get no errors
		err := createLockTableIfNecessary(tableName, client)
		assert.Nil(t, err)
	})
}

func TestWriteItemToLockTable(t *testing.T) {
	t.Parallel()

	// First, create a table
	withLockTable(t, func(tableName string, client *dynamodb.DynamoDB) {
		itemId := uniqueId()

		// Now write an item to the table
		err := writeItemToLockTable(itemId, tableName, client)
		assert.Nil(t, err)

		// Finally, check the item exists
		assertItemExistsInTable(t, itemId, tableName, client)
	})
}

func TestWriteAndRemoveItemFromLockTable(t *testing.T) {
	t.Parallel()

	// First, create a table
	withLockTable(t, func(tableName string, client *dynamodb.DynamoDB) {
		itemId := uniqueId()

		// Now write an item to the table
		err := writeItemToLockTable(itemId, tableName, client)
		assert.Nil(t, err)

		// Next, check the item exists
		assertItemExistsInTable(t, itemId, tableName, client)

		// Now remove the item
		err = removeItemFromLockTable(itemId, tableName, client)
		assert.Nil(t, err)

		// Finally, check the item no longer exists
		assertItemNotExistsInTable(t, itemId, tableName, client)
	})
}

func TestWriteItemToLockTableUntilSuccessItemDoesntAlreadyExist(t *testing.T) {
	t.Parallel()

	// First, create a table
	withLockTable(t, func(tableName string, client *dynamodb.DynamoDB) {
		itemId := uniqueId()

		// Now write an item to the table. Allow no retries, as the item shouldn't already exit.
		err := writeItemToLockTableUntilSuccess(itemId, tableName, client, 1, 1 * time.Millisecond)
		assert.Nil(t, err)

		// Finally, check the item exists
		assertItemExistsInTable(t, itemId, tableName, client)
	})
}

func TestWriteItemToLockTableUntilSuccessItemAlreadyExists(t *testing.T) {
	t.Parallel()

	// First, create a table
	withLockTable(t, func(tableName string, client *dynamodb.DynamoDB) {
		itemId := uniqueId()

		// Now write an item to the table
		err := writeItemToLockTable(itemId, tableName, client)
		assert.Nil(t, err)

		// Check the item exists
		assertItemExistsInTable(t, itemId, tableName, client)

		// Now try to write the item to the table again. Allow no retries to ensure this fails immediately.
		err = writeItemToLockTableUntilSuccess(itemId, tableName, client, 1, 1 * time.Millisecond)
		assert.True(t, errors.IsError(err, AcquireLockRetriesExceeded{ItemId: itemId, Retries: 1}), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
	})
}

func TestWriteItemToLockTableUntilSuccessItemAlreadyExistsButGetsDeleted(t *testing.T) {
	t.Parallel()

	// First, create a table
	withLockTable(t, func(tableName string, client *dynamodb.DynamoDB) {
		itemId := uniqueId()

		// Now write an item to the table
		err := writeItemToLockTable(itemId, tableName, client)
		assert.Nil(t, err)

		// Check the item exists
		assertItemExistsInTable(t, itemId, tableName, client)

		// Launch a goroutine in the background to delete this item after 30 seconds
		go func() {
			time.Sleep(30 * time.Second)
			err := removeItemFromLockTable(itemId, tableName, client)
			assert.Nil(t, err)
		}()

		// In the meantime, try to write the item to the table again. This should fail initially, so allow 18
		// retries. At 10 seconds per retry, that's 3 minutes, which should be enough time for the goroutine to
		// delete the item and for that info to make it to the majority of the DynamoDB nodes.
		err = writeItemToLockTableUntilSuccess(itemId, tableName, client, 18, 10 * time.Second)
		assert.Nil(t, err, "Got unexpected error: %v", err)
	})
}

func TestWriteItemToLockTableConcurrency(t * testing.T) {
	t.Parallel()

	concurrency := 20

	// First, create a table with enough read and write units to ensure this test doesn't get throttled
	withLockTableProvisionedUnits(t, concurrency, concurrency, func(tableName string, client *dynamodb.DynamoDB) {
		itemId := uniqueId()

		// Use a WaitGroup to ensure the test doesn't exit before all goroutines finish.
		var waitGroup sync.WaitGroup
		// This will count how many of the goroutines succeeded in writing an item (we expect only 1 to
		// succeed). We use Go's atomic package to ensure all modifications to this counter are atomic
		// operations.
		successfulWrites := int32(0)

		// Launch a bunch of goroutines who will all try to create the same item at more or less the same time.
		// Only one of the goroutines should succeed.
		for i := 0; i < concurrency; i++ {
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				err := writeItemToLockTable(itemId, tableName, client)
				if err == nil {
					atomic.AddInt32(&successfulWrites, 1)
				} else {
					assert.True(t, isItemAlreadyExistsErr(err), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
				}
			}()
		}

		waitGroup.Wait()

		assert.Equal(t, int32(1), successfulWrites, "Only one of the goroutines should have been able to successfully write an item")
	})
}