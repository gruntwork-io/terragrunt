package dynamodb

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"time"
)

func TestCreateLockTableIfNecessaryTableDoesntAlreadyExist(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	tableName := uniqueTableNameForTest()

	err := createLockTableIfNecessary(tableName, client)
	defer cleanupTable(t, tableName, client)

	assert.Nil(t, err)
	assertCanWriteToTable(t, tableName, client)
}

func TestCreateLockTableIfNecessaryTableAlreadyExists(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	tableName := uniqueTableNameForTest()

	// Create the table the first time
	err := createLockTableIfNecessary(tableName, client)
	defer cleanupTable(t, tableName, client)

	assert.Nil(t, err)
	assertCanWriteToTable(t, tableName, client)

	// Try to create the table the second time and make sure you get no errors
	err = createLockTableIfNecessary(tableName, client)
	assert.Nil(t, err)
}

func TestWriteItemToLockTable(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	itemId := uniqueId()
	tableName := uniqueTableNameForTest()

	// First, create a table
	err := createLockTableIfNecessary(tableName, client)
	defer cleanupTable(t, tableName, client)
	assert.Nil(t, err)

	// Now write an item to the table
	err = writeItemToLockTable(itemId, tableName, client)
	assert.Nil(t, err)

	// Finally, check the item exists
	assertItemExistsInTable(t, itemId, tableName, client)
}

func TestWriteAndRemoveItemFromLockTable(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	itemId := uniqueId()
	tableName := uniqueTableNameForTest()

	// First, create a table
	err := createLockTableIfNecessary(tableName, client)
	defer cleanupTable(t, tableName, client)
	assert.Nil(t, err)

	// Now write an item to the table
	err = writeItemToLockTable(itemId, tableName, client)
	assert.Nil(t, err)

	// Next, check the item exists
	assertItemExistsInTable(t, itemId, tableName, client)

	// Now remove the item
	err = removeItemFromLockTable(itemId, tableName, client)
	assert.Nil(t, err)

	// Finally, check the item no longer exists
	assertItemNotExistsInTable(t, itemId, tableName, client)
}

func TestWriteItemToLockTableUntilSuccessItemDoesntAlreadyExist(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	itemId := uniqueId()
	tableName := uniqueTableNameForTest()

	// First, create a table
	err := createLockTableIfNecessary(tableName, client)
	defer cleanupTable(t, tableName, client)
	assert.Nil(t, err)

	// Now write an item to the table. Allow no retries, as the item shouldn't already.
	err = writeItemToLockTableUntilSuccess(itemId, tableName, client, 1)
	assert.Nil(t, err)

	// Finally, check the item exists
	assertItemExistsInTable(t, itemId, tableName, client)
}

func TestWriteItemToLockTableUntilSuccessItemAlreadyExists(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	itemId := uniqueId()
	tableName := uniqueTableNameForTest()

	// First, create a table
	err := createLockTableIfNecessary(tableName, client)
	defer cleanupTable(t, tableName, client)
	assert.Nil(t, err)

	// Now write an item to the table
	err = writeItemToLockTable(itemId, tableName, client)
	assert.Nil(t, err)

	// Check the item exists
	assertItemExistsInTable(t, itemId, tableName, client)

	// Now try to write the item to the table again. Allow no retries to ensure this fails immediately.
	err = writeItemToLockTableUntilSuccess(itemId, tableName, client, 1)
	assert.NotNil(t, err)
}

func TestWriteItemToLockTableUntilSuccessItemAlreadyExistsButGetsDeleted(t *testing.T) {
	t.Parallel()

	client := createDynamoDbClientForTest(t)
	itemId := uniqueId()
	tableName := uniqueTableNameForTest()

	// First, create a table
	err := createLockTableIfNecessary(tableName, client)
	defer cleanupTable(t, tableName, client)
	assert.Nil(t, err)

	// Now write an item to the table
	err = writeItemToLockTable(itemId, tableName, client)
	assert.Nil(t, err)

	// Check the item exists
	assertItemExistsInTable(t, itemId, tableName, client)

	// Launch a goroutine in the background to delete this item after 30 seconds
	go func() {
		time.Sleep(30 * time.Second)
		err := removeItemFromLockTable(itemId, tableName, client)
		assert.Nil(t, err)
	}()

	// In the meantime, try to write the item to the table again. This should fail initially, so allow 18 retries.
	// At 10 seconds per retry, that's 3 minutes, which should be enough time for the goroutine to delete the item
	// and for that info to make it to the majority of the DynamoDB nodes.
	err = writeItemToLockTableUntilSuccess(itemId, tableName, client, 18)
	assert.Nil(t, err)
}

