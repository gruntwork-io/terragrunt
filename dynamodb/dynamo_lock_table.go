// Package dynamodb provides utilities for working with DynamoDB tables, such as creating and deleting tables.
package dynamodb

import (
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/gruntwork-io/terragrunt/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// DynamoDB only allows 10 table creates/deletes simultaneously. To ensure we don't hit this error, especially when
// running many automated tests in parallel, we use a counting semaphore
const dynamoParallelOperations = 10

var tableCreateDeleteSemaphore = NewCountingSemaphore(dynamoParallelOperations)

// AttrLockID is the name of the primary key for the lock table in DynamoDB.
// OpenTofu/Terraform requires the DynamoDB table to have a primary key with this name
const AttrLockID = "LockID"

// MaxRetriesWaitingForTableToBeActive is the maximum number of times we
// will retry waiting for a table to be active.
//
// Default is to retry for up to 5 minutes
const MaxRetriesWaitingForTableToBeActive = 30

// SleepBetweenTableStatusChecks is the amount of time we will sleep between
// checks to see if a table is active.
const SleepBetweenTableStatusChecks = 10 * time.Second

// DynamodbPayPerRequestBillingMode is the billing mode for DynamoDB tables that allows for pay-per-request billing
// instead of provisioned capacity.
const DynamodbPayPerRequestBillingMode = "PAY_PER_REQUEST"

const sleepBetweenRetries = 20 * time.Second
const maxRetries = 15

// CreateDynamoDBClient creates an authenticated client for DynamoDB.
func CreateDynamoDBClient(config *awshelper.AwsSessionConfig, terragruntOptions *options.TerragruntOptions) (*dynamodb.DynamoDB, error) {
	session, err := awshelper.CreateAwsSession(config, terragruntOptions)
	if err != nil {
		return nil, err
	}

	return dynamodb.New(session), nil
}

// CreateLockTableIfNecessary creates the lock table in DynamoDB if it doesn't already exist.
func CreateLockTableIfNecessary(tableName string, tags map[string]string, client *dynamodb.DynamoDB, terragruntOptions *options.TerragruntOptions) error {
	tableExists, err := LockTableExistsAndIsActive(tableName, client)
	if err != nil {
		return err
	}

	if !tableExists {
		terragruntOptions.Logger.Debugf("Lock table %s does not exist in DynamoDB. Will need to create it just this first time.", tableName)
		return CreateLockTable(tableName, tags, client, terragruntOptions)
	}

	return nil
}

// LockTableExistsAndIsActive returns true if the lock table exists in DynamoDB and is in "active" state.
func LockTableExistsAndIsActive(tableName string, client *dynamodb.DynamoDB) (bool, error) {
	output, err := client.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(tableName)})
	if err != nil {
		var awsErr awserr.Error
		if ok := errors.As(err, &awsErr); ok && awsErr.Code() == "ResourceNotFoundException" {
			return false, nil
		} else {
			return false, errors.New(err)
		}
	}

	return *output.Table.TableStatus == dynamodb.TableStatusActive, nil
}

// LockTableCheckSSEncryptionIsOn returns true if the lock table's SSEncryption is turned on
func LockTableCheckSSEncryptionIsOn(tableName string, client *dynamodb.DynamoDB) (bool, error) {
	output, err := client.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(tableName)})
	if err != nil {
		return false, errors.New(err)
	}

	return output.Table.SSEDescription != nil && aws.StringValue(output.Table.SSEDescription.Status) == dynamodb.SSEStatusEnabled, nil
}

// CreateLockTable creates a lock table in DynamoDB and wait until it is in "active" state.
// If the table already exists, merely wait until it is in "active" state.
func CreateLockTable(tableName string, tags map[string]string, client *dynamodb.DynamoDB, terragruntOptions *options.TerragruntOptions) error {
	tableCreateDeleteSemaphore.Acquire()
	defer tableCreateDeleteSemaphore.Release()

	terragruntOptions.Logger.Debugf("Creating table %s in DynamoDB", tableName)

	attributeDefinitions := []*dynamodb.AttributeDefinition{
		{AttributeName: aws.String(AttrLockID), AttributeType: aws.String(dynamodb.ScalarAttributeTypeS)},
	}

	keySchema := []*dynamodb.KeySchemaElement{
		{AttributeName: aws.String(AttrLockID), KeyType: aws.String(dynamodb.KeyTypeHash)},
	}

	createTableOutput, err := client.CreateTable(&dynamodb.CreateTableInput{
		TableName:            aws.String(tableName),
		BillingMode:          aws.String(DynamodbPayPerRequestBillingMode),
		AttributeDefinitions: attributeDefinitions,
		KeySchema:            keySchema,
	})

	if err != nil {
		if isTableAlreadyBeingCreatedOrUpdatedError(err) {
			terragruntOptions.Logger.Debugf("Looks like someone created table %s at the same time. Will wait for it to be in active state.", tableName)
		} else {
			return errors.New(err)
		}
	}

	err = waitForTableToBeActive(tableName, client, MaxRetriesWaitingForTableToBeActive, SleepBetweenTableStatusChecks, terragruntOptions)

	if err != nil {
		return err
	}

	if createTableOutput != nil && createTableOutput.TableDescription != nil && createTableOutput.TableDescription.TableArn != nil {
		// Do not tag in case somebody else had created the table
		err = tagTableIfTagsGiven(tags, createTableOutput.TableDescription.TableArn, client, terragruntOptions)

		if err != nil {
			return errors.New(err)
		}
	}

	return nil
}

func tagTableIfTagsGiven(tags map[string]string, tableArn *string, client *dynamodb.DynamoDB, terragruntOptions *options.TerragruntOptions) error {
	if len(tags) == 0 {
		terragruntOptions.Logger.Debugf("No tags for lock table given.")
		return nil
	}

	// we were able to create the table successfully, now add tags
	terragruntOptions.Logger.Debugf("Adding tags to lock table: %s", tags)

	var tagsConverted = make([]*dynamodb.Tag, 0, len(tags))

	for k, v := range tags {
		tagsConverted = append(tagsConverted, &dynamodb.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	var input = dynamodb.TagResourceInput{
		ResourceArn: tableArn,
		Tags:        tagsConverted}

	_, err := client.TagResource(&input)

	return err
}

// DeleteTable deletes the given table in DynamoDB.
func DeleteTable(tableName string, dbClient *dynamodb.DynamoDB) error {
	const (
		maxRetries    = 5
		minRetryDelay = time.Second
	)

	tableCreateDeleteSemaphore.Acquire()
	defer tableCreateDeleteSemaphore.Release()

	req, _ := dbClient.DeleteTableRequest(&dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
	// It is not always able to delete a table the first attempt, error: `StatusCode: 400, Attempt to change a resource which is still in use: Table tags are being updated: terragrunt_test_*`
	req.Retryer = &DeleteTableRetryer{DefaultRetryer: client.DefaultRetryer{
		NumMaxRetries: maxRetries,
		MinRetryDelay: minRetryDelay,
	}}

	return req.Send()
}

type DeleteTableRetryer struct {
	client.DefaultRetryer
}

func (retryer DeleteTableRetryer) ShouldRetry(req *request.Request) bool {
	if req.HTTPResponse.StatusCode == http.StatusBadRequest {
		return true
	}

	return retryer.DefaultRetryer.ShouldRetry(req)
}

// Return true if the given error is the error message returned by AWS when the resource already exists and is being
// updated by someone else
func isTableAlreadyBeingCreatedOrUpdatedError(err error) bool {
	var awsErr awserr.Error
	ok := errors.As(err, &awsErr)

	return ok && awsErr.Code() == "ResourceInUseException"
}

// Wait for the given DynamoDB table to be in the "active" state. If it's not in "active" state, sleep for the
// specified amount of time, and try again, up to a maximum of maxRetries retries.
func waitForTableToBeActive(tableName string, client *dynamodb.DynamoDB, maxRetries int, sleepBetweenRetries time.Duration, terragruntOptions *options.TerragruntOptions) error {
	return WaitForTableToBeActiveWithRandomSleep(tableName, client, maxRetries, sleepBetweenRetries, sleepBetweenRetries, terragruntOptions)
}

// WaitForTableToBeActiveWithRandomSleep waits for the given table as described above,
// but sleeps a random amount of time greater than sleepBetweenRetriesMin
// and less than sleepBetweenRetriesMax between tries. This is to avoid an AWS issue where all waiting requests fire at
// the same time, which continually triggered AWS's "subscriber limit exceeded" API error.
func WaitForTableToBeActiveWithRandomSleep(tableName string, client *dynamodb.DynamoDB, maxRetries int, sleepBetweenRetriesMin time.Duration, sleepBetweenRetriesMax time.Duration, terragruntOptions *options.TerragruntOptions) error {
	for i := 0; i < maxRetries; i++ {
		tableReady, err := LockTableExistsAndIsActive(tableName, client)
		if err != nil {
			return err
		}

		if tableReady {
			terragruntOptions.Logger.Debugf("Success! Table %s is now in active state.", tableName)
			return nil
		}

		sleepBetweenRetries := util.GetRandomTime(sleepBetweenRetriesMin, sleepBetweenRetriesMax)
		terragruntOptions.Logger.Debugf("Table %s is not yet in active state. Will check again after %s.", tableName, sleepBetweenRetries)
		time.Sleep(sleepBetweenRetries)
	}

	return errors.New(TableActiveRetriesExceeded{TableName: tableName, Retries: maxRetries})
}

// UpdateLockTableSetSSEncryptionOnIfNecessary encrypts the TFState Lock table - If Necessary
func UpdateLockTableSetSSEncryptionOnIfNecessary(tableName string, client *dynamodb.DynamoDB, terragruntOptions *options.TerragruntOptions) error {
	tableSSEncrypted, err := LockTableCheckSSEncryptionIsOn(tableName, client)
	if err != nil {
		return errors.New(err)
	}

	if tableSSEncrypted {
		terragruntOptions.Logger.Debugf("Table %s already has encryption enabled", tableName)
		return nil
	}

	tableCreateDeleteSemaphore.Acquire()
	defer tableCreateDeleteSemaphore.Release()

	terragruntOptions.Logger.Debugf("Enabling server-side encryption on table %s in AWS DynamoDB", tableName)

	input := &dynamodb.UpdateTableInput{
		SSESpecification: &dynamodb.SSESpecification{
			Enabled: aws.Bool(true),
			SSEType: aws.String("KMS"),
		},
		TableName: aws.String(tableName),
	}

	if _, err := client.UpdateTable(input); err != nil {
		if isTableAlreadyBeingCreatedOrUpdatedError(err) {
			terragruntOptions.Logger.Debugf("Looks like someone is already updating table %s at the same time. Will wait for that update to complete.", tableName)
		} else {
			return errors.New(err)
		}
	}

	if err := waitForEncryptionToBeEnabled(tableName, client, terragruntOptions); err != nil {
		return errors.New(err)
	}

	return waitForTableToBeActive(tableName, client, MaxRetriesWaitingForTableToBeActive, SleepBetweenTableStatusChecks, terragruntOptions)
}

// Wait until encryption is enabled for the given table
func waitForEncryptionToBeEnabled(tableName string, client *dynamodb.DynamoDB, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Debugf("Waiting for encryption to be enabled on table %s", tableName)

	for i := 0; i < maxRetries; i++ {
		tableSSEncrypted, err := LockTableCheckSSEncryptionIsOn(tableName, client)
		if err != nil {
			return errors.New(err)
		}

		if tableSSEncrypted {
			terragruntOptions.Logger.Debugf("Encryption is now enabled for table %s!", tableName)
			return nil
		}

		terragruntOptions.Logger.Debugf("Encryption is still not enabled for table %s. Will sleep for %v and try again.", tableName, sleepBetweenRetries)
		time.Sleep(sleepBetweenRetries)
	}

	return errors.New(TableEncryptedRetriesExceeded{TableName: tableName, Retries: maxRetries})
}

// Custom error types

type TableActiveRetriesExceeded struct {
	TableName string
	Retries   int
}

func (err TableActiveRetriesExceeded) Error() string {
	return fmt.Sprintf("Table %s is still not in active state after %d retries.", err.TableName, err.Retries)
}

type TableDoesNotExist struct {
	TableName  string
	Underlying error
}

func (err TableDoesNotExist) Error() string {
	return fmt.Sprintf("Table %s does not exist in DynamoDB! Original error from AWS: %v", err.TableName, err.Underlying)
}

type TableEncryptedRetriesExceeded struct {
	TableName string
	Retries   int
}

func (err TableEncryptedRetriesExceeded) Error() string {
	return fmt.Sprintf("Table %s still does not have encryption enabled after %d retries.", err.TableName, err.Retries)
}
