package dynamodb

import (
	"testing"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"time"
	"github.com/aws/aws-sdk-go/aws"
)

func TestToLockMetadata(t *testing.T) {
	t.Parallel()

	itemId := "item-id"
	username := "username"
	ip := "11.22.33.44"
	creationDate := time.Now().UTC()

	attributes := map[string]*dynamodb.AttributeValue{
		ATTR_USERNAME: &dynamodb.AttributeValue{S: aws.String(username)},
		ATTR_IP: &dynamodb.AttributeValue{S: aws.String(ip)},
		ATTR_CREATION_DATE: &dynamodb.AttributeValue{S: aws.String(creationDate.String())},
	}

	lockMetadata, err := toLockMetadata(itemId, attributes)

	assert.Nil(t, err)
	assert.Equal(t, itemId, lockMetadata.StateFileId)
	assert.Equal(t, username, lockMetadata.Username)
	assert.Equal(t, ip, lockMetadata.IpAddress)
	assert.Equal(t, creationDate, lockMetadata.DateCreated)
}

func TestToLockMetadataInvalidCreationDate(t *testing.T) {
	t.Parallel()

	itemId := "item-id"
	username := "username"
	ip := "11.22.33.44"

	attributes := map[string]*dynamodb.AttributeValue{
		ATTR_USERNAME: &dynamodb.AttributeValue{S: aws.String(username)},
		ATTR_IP: &dynamodb.AttributeValue{S: aws.String(ip)},
		ATTR_CREATION_DATE: &dynamodb.AttributeValue{S: aws.String("not-a-valid-date")},
	}

	_, err := toLockMetadata(itemId, attributes)

	assert.NotNil(t, err)
}

func TestToLockMetadataEmpty(t *testing.T) {
	t.Parallel()

	itemId := "item-id"
	attributes := map[string]*dynamodb.AttributeValue{}

	_, err := toLockMetadata(itemId, attributes)
	assert.NotNil(t, err)
}
