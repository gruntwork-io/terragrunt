package aws_helper

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"time"
)

// Returns an AWS session object for the given region (required), profile name (optional), and IAM role to assume
// (optional), ensuring that the credentials are available
func CreateAwsSession(awsRegion, awsEndpoint string, awsProfile string, iamRoleArn string, terragruntOptions *options.TerragruntOptions) (*session.Session, error) {
	defaultResolver := endpoints.DefaultResolver()
	s3CustResolverFn := func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		if service == "s3" && awsEndpoint != "" {
			return endpoints.ResolvedEndpoint{
				URL:           awsEndpoint,
				SigningRegion: awsRegion,
			}, nil
		}

		return defaultResolver.EndpointFor(service, region, optFns...)
	}

	var awsConfig = aws.Config{
		Region:           aws.String(awsRegion),
		EndpointResolver: endpoints.ResolverFunc(s3CustResolverFn),
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Config:            awsConfig,
		Profile:           awsProfile,
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error initializing session")
	}

	if iamRoleArn != "" {
		sess.Config.Credentials = stscreds.NewCredentials(sess, iamRoleArn)
	} else if terragruntOptions.IamRole != "" {
		sess.Config.Credentials = stscreds.NewCredentials(sess, terragruntOptions.IamRole)
	}

	_, err = sess.Config.Credentials.Get()
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?)")
	}

	return sess, nil
}

// Make API calls to AWS to assume the IAM role specified and return the temporary AWS credentials to use that role
func AssumeIamRole(iamRoleArn string) (*sts.Credentials, error) {
	sess, err := session.NewSession()
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	_, err = sess.Config.Credentials.Get()
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?)")
	}

	stsClient := sts.New(sess)

	input := sts.AssumeRoleInput{
		RoleArn:         aws.String(iamRoleArn),
		RoleSessionName: aws.String(fmt.Sprintf("terragrunt-%d", time.Now().UTC().UnixNano())),
	}

	output, err := stsClient.AssumeRole(&input)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return output.Credentials, nil
}
