package dynamodb

import (
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/locks"
	"time"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/aws/aws-sdk-go/aws"
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
)

// Create a DynamoDB key for the given item id
func createKeyFromItemId(itemId string) map[string]*dynamodb.AttributeValue {
	return map[string]*dynamodb.AttributeValue {
		ATTR_STATE_FILE_ID: &dynamodb.AttributeValue{S: aws.String(itemId)},
	}
}

// Fetch the metadata for the given item from DynamoDB and display it to stdout. This metadata will contain info about
// who currently has the lock.
func displayLockMetadata(itemId string, tableName string, client *dynamodb.DynamoDB) {
	lockMetadata, err := getLockMetadata(itemId, tableName, client)
	if err != nil {
		util.Logger.Printf("Someone already has a lock on state file %s in table %s in DynamoDB! However, failed to fetch metadata for the lock (perhaps the lock has since been released?): %s", itemId, tableName, err.Error())
	} else {
		util.Logger.Printf("Someone already has a lock on state file %s! %s@%s acquired the lock on %s.", itemId, lockMetadata.Username, lockMetadata.IpAddress, lockMetadata.DateCreated.String())
	}
}

// Fetch the lock metadata for the given item from DynamoDB. This metadata will contain info about who currently has
// the lock.
func getLockMetadata(itemId string, tableName string, client *dynamodb.DynamoDB) (*locks.LockMetadata, error) {
	output, err := client.GetItem(&dynamodb.GetItemInput{
		Key: createKeyFromItemId(itemId),
		ConsistentRead: aws.Bool(true),
		TableName: aws.String(tableName),
	})

	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return toLockMetadata(itemId, output.Item)
}

// Convert the AttributeValue map returned by DynamoDB into a LockMetadata struct
func toLockMetadata(itemId string, item map[string]*dynamodb.AttributeValue) (*locks.LockMetadata, error) {
	username, err := getAttribute(item, ATTR_USERNAME)
	if err != nil {
		return nil, err
	}

	ipAddress, err := getAttribute(item, ATTR_IP)
	if err != nil {
		return nil, err
	}

	dateCreatedStr, err := getAttribute(item, ATTR_CREATION_DATE)
	if err != nil {
		return nil, err
	}

	dateCreated, err := time.Parse(locks.DEFAULT_TIME_FORMAT, dateCreatedStr)
	if err != nil {
		return nil, errors.WithStackTrace(InvalidDateFormat{Date: dateCreatedStr, UnderlyingErr: err})
	}

	return &locks.LockMetadata{
		StateFileId: itemId,
		Username: username,
		IpAddress: ipAddress,
		DateCreated: dateCreated,
	}, nil
}

// Return the value for the given attribute from the given attribute map, or return an error if that attribute is
// missing from the map
func getAttribute(item map[string]*dynamodb.AttributeValue, attribute string) (string, error) {
	value, exists := item[attribute]
	if !exists {
		return "", errors.WithStackTrace(AttributeMissing{AttributeName: attribute})
	}

	return *value.S, nil
}

// Create a DynamoDB item for the given item id. This item represents a lock and will include metadata about the
// current user, who is trying to acquire the lock.
func createItemAttributes(itemId string, client *dynamodb.DynamoDB) (map[string]*dynamodb.AttributeValue, error) {
	callerIdentity, err := getCallerIdentity(client)
	if err != nil {
		return nil, err
	}

	lockMetadata, err := locks.CreateLockMetadata(itemId, callerIdentity)
	if err != nil {
		return nil, err
	}

	return map[string]*dynamodb.AttributeValue{
		ATTR_STATE_FILE_ID: &dynamodb.AttributeValue{S: aws.String(itemId)},
		ATTR_USERNAME: &dynamodb.AttributeValue{S: aws.String(lockMetadata.Username)},
		ATTR_IP: &dynamodb.AttributeValue{S: aws.String(lockMetadata.IpAddress)},
		ATTR_CREATION_DATE: &dynamodb.AttributeValue{S: aws.String(lockMetadata.DateCreated.String())},
	}, nil
}

// Return the UserID
func getCallerIdentity(client *dynamodb.DynamoDB) (string, error) {
	stsconn := sts.New(session.New(), &client.Config)
	output, err := stsconn.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return *output.UserId, nil
}

type AttributeMissing struct {
	AttributeName string
}

func (err AttributeMissing) Error() string {
	return fmt.Sprintf("Could not find attribute %s", err.AttributeName)
}

type InvalidDateFormat struct {
	Date 		string
	UnderlyingErr 	error
}

func (err InvalidDateFormat) Error() string {
	return fmt.Sprintf("Unable to parse date %s: %s", err.Date, err.UnderlyingErr.Error())
}