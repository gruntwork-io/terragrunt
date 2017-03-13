package aws_helper

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/gruntwork-io/terragrunt/errors"
)

// Returns an AWS session object for the given region, ensuring that the credentials are available
func CreateAwsSession(awsRegion, awsProfile, awsRoleArn string) (*session.Session, error) {
	session, err := session.NewSessionWithOptions(session.Options{
		Config:            aws.Config{Region: aws.String(awsRegion)},
		Profile:           awsProfile,
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error intializing session")
	}

	_, err = session.Config.Credentials.Get()
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?)")
	}

	if awsRoleArn != "" {
		err = assumeRole(session, awsRoleArn)
		if err != nil {
			return nil, errors.WithStackTraceAndPrefix(err, "Error assuming given AWS role")
		}
	}

	return session, nil
}

func assumeRole(s *session.Session, awsRoleArn string) (err error) {
	client := sts.New(s)
	assumeRoleProvider := &stscreds.AssumeRoleProvider{
		Client:  client,
		RoleARN: awsRoleArn,
	}
	credentials := credentials.NewChainCredentials([]credentials.Provider{assumeRoleProvider})

	_, err = credentials.Get()
	if err != nil {
		return err
	}

	s.Config.WithCredentials(credentials)
	return nil
}
