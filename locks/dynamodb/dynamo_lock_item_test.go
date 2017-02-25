package dynamodb

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
	"time"
)

func TestToLockMetadata(t *testing.T) {
	t.Parallel()

	itemId := "item-id"
	username := "username"
	ip := "11.22.33.44"
	creationDate := time.Now().UTC()

	attributes := map[string]*dynamodb.AttributeValue{
		ATTR_USERNAME:      &dynamodb.AttributeValue{S: aws.String(username)},
		ATTR_IP:            &dynamodb.AttributeValue{S: aws.String(ip)},
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
	invalidDate := "not-a-valid-date"

	attributes := map[string]*dynamodb.AttributeValue{
		ATTR_USERNAME:      &dynamodb.AttributeValue{S: aws.String(username)},
		ATTR_IP:            &dynamodb.AttributeValue{S: aws.String(ip)},
		ATTR_CREATION_DATE: &dynamodb.AttributeValue{S: aws.String(invalidDate)},
	}

	_, err := toLockMetadata(itemId, attributes)
	assert.NotNil(t, err)

	underlying := errors.Unwrap(err)
	invalidDateFormat, isInvalidDateFormat := underlying.(InvalidDateFormat)

	assert.True(t, isInvalidDateFormat, "Unexpected error of type %s: %s", reflect.TypeOf(underlying), underlying)
	assert.Equal(t, invalidDate, invalidDateFormat.Date)
}

func TestToLockMetadataMissingUsername(t *testing.T) {
	t.Parallel()

	itemId := "item-id"
	ip := "11.22.33.44"

	attributes := map[string]*dynamodb.AttributeValue{
		ATTR_IP:            &dynamodb.AttributeValue{S: aws.String(ip)},
		ATTR_CREATION_DATE: &dynamodb.AttributeValue{S: aws.String("not-a-valid-date")},
	}

	_, err := toLockMetadata(itemId, attributes)
	assert.True(t, errors.IsError(err, AttributeMissing{AttributeName: ATTR_USERNAME}), "Unexpected error of type %s: %s", reflect.TypeOf(err), err)
}
