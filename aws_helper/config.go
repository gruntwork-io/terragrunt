package aws_helper

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/gruntwork-io/terragrunt/errors"
)

// Returns an AWS session object for the given region (required), profile name (optional), and IAM role to assume
// (optional), ensuring that the credentials are available
func CreateAwsSession(awsRegion, awsProfile string, iamRoleArn string) (*session.Session, error) {
	sess, err := session.NewSessionWithOptions(session.Options{
		Config:            aws.Config{Region: aws.String(awsRegion)},
		Profile:           awsProfile,
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error initializing session")
	}

	if iamRoleArn != "" {
		sess.Config.Credentials = stscreds.NewCredentials(sess, iamRoleArn)
	}

	_, err = sess.Config.Credentials.Get()
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?)")
	}

	return sess, nil
}
