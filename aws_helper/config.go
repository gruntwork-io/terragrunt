package aws_helper

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

// A representation of the configuration options for an AWS Session
type AwsSessionConfig struct {
	Region                  string
	CustomS3Endpoint        string
	CustomDynamoDBEndpoint  string
	Profile                 string
	RoleArn                 string
	CredsFilename           string
	S3ForcePathStyle        bool
	DisableComputeChecksums bool
	ExternalID              string
	SessionName             string
}

// Returns an AWS session object for the given config region (required), profile name (optional), and IAM role to assume
// (optional), ensuring that the credentials are available.
func CreateAwsSessionFromConfig(config *AwsSessionConfig, terragruntOptions *options.TerragruntOptions) (*session.Session, error) {
	defaultResolver := endpoints.DefaultResolver()
	s3CustResolverFn := func(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		if service == "s3" && config.CustomS3Endpoint != "" {
			return endpoints.ResolvedEndpoint{
				URL:           config.CustomS3Endpoint,
				SigningRegion: config.Region,
			}, nil
		} else if service == "dynamodb" && config.CustomDynamoDBEndpoint != "" {
			return endpoints.ResolvedEndpoint{
				URL:           config.CustomDynamoDBEndpoint,
				SigningRegion: config.Region,
			}, nil
		}

		return defaultResolver.EndpointFor(service, region, optFns...)
	}

	var awsConfig = aws.Config{
		Region:                  aws.String(config.Region),
		EndpointResolver:        endpoints.ResolverFunc(s3CustResolverFn),
		S3ForcePathStyle:        aws.Bool(config.S3ForcePathStyle),
		DisableComputeChecksums: aws.Bool(config.DisableComputeChecksums),
	}

	var sessionOptions = session.Options{
		Config:            awsConfig,
		Profile:           config.Profile,
		SharedConfigState: session.SharedConfigEnable,
	}

	if len(config.CredsFilename) > 0 {
		sessionOptions.SharedConfigFiles = []string{config.CredsFilename}
	}

	sess, err := session.NewSessionWithOptions(sessionOptions)
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error initializing session")
	}

	credentialsOptFn := func(p *stscreds.AssumeRoleProvider) {
		if config.ExternalID != "" {
			p.ExternalID = aws.String(config.ExternalID)
		}
		if config.SessionName != "" {
			p.RoleSessionName = config.SessionName
		}
	}

	if config.RoleArn != "" {
		sess.Config.Credentials = stscreds.NewCredentials(sess, config.RoleArn, credentialsOptFn)
	} else if terragruntOptions.IamRole != "" {
		sess.Config.Credentials = stscreds.NewCredentials(sess, terragruntOptions.IamRole, credentialsOptFn)
	}
	return sess, nil
}

// Returns an AWS session object. The session is configured by either:
// - The provided AwsSessionConfig struct, which specifies region (required), profile name (optional), and IAM role to
//   assume (optional).
// - The provided TerragruntOptions struct, which specifies any IAM role to assume (optional).
// Note that if the AwsSessionConfig object is null, this will return default session credentials using the default
// credentials chain of the AWS SDK.
func CreateAwsSession(config *AwsSessionConfig, terragruntOptions *options.TerragruntOptions) (*session.Session, error) {
	var sess *session.Session
	var err error
	if config == nil {
		sess, err = session.NewSession()
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		if terragruntOptions.IamRole != "" {
			sess.Config.Credentials = stscreds.NewCredentials(sess, terragruntOptions.IamRole)
		}
	} else {
		sess, err = CreateAwsSessionFromConfig(config, terragruntOptions)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
	}

	if _, err = sess.Config.Credentials.Get(); err != nil {
		msg := "Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?)"
		if config != nil && len(config.CredsFilename) > 0 {
			msg = fmt.Sprintf("Error finding AWS credentials in file '%s' (did you set the correct file name and/or profile?)", config.CredsFilename)
		}

		return nil, errors.WithStackTraceAndPrefix(err, msg)
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

// Return the AWS caller identity associated with the current set of credentials
func GetAWSCallerIdentity(config *AwsSessionConfig, terragruntOptions *options.TerragruntOptions) (sts.GetCallerIdentityOutput, error) {
	sess, err := CreateAwsSession(config, terragruntOptions)
	if err != nil {
		return sts.GetCallerIdentityOutput{}, errors.WithStackTrace(err)
	}

	identity, err := sts.New(sess).GetCallerIdentity(nil)
	if err != nil {
		return sts.GetCallerIdentityOutput{}, errors.WithStackTrace(err)
	}

	return *identity, nil
}

// Get the AWS account ID of the current session configuration
func GetAWSAccountID(config *AwsSessionConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	identity, err := GetAWSCallerIdentity(config, terragruntOptions)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return *identity.Account, nil
}

// Get the ARN of the AWS identity associated with the current set of credentials
func GetAWSIdentityArn(config *AwsSessionConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	identity, err := GetAWSCallerIdentity(config, terragruntOptions)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return *identity.Arn, nil
}

// Get the AWS user ID of the current session configuration
func GetAWSUserID(config *AwsSessionConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	identity, err := GetAWSCallerIdentity(config, terragruntOptions)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return *identity.UserId, nil
}

// Assume an IAM role, if one is specified, by making API calls to Amazon STS and setting the environment variables
// we get back inside of terragruntOptions.Env
func AssumeRoleAndUpdateEnvIfNecessary(terragruntOptions *options.TerragruntOptions) error {
	if terragruntOptions.IamRole == "" {
		return nil
	}

	terragruntOptions.Logger.Debugf("Assuming IAM role %s", terragruntOptions.IamRole)
	creds, err := AssumeIamRole(terragruntOptions.IamRole)
	if err != nil {
		return err
	}

	terragruntOptions.Env["AWS_ACCESS_KEY_ID"] = aws.StringValue(creds.AccessKeyId)
	terragruntOptions.Env["AWS_SECRET_ACCESS_KEY"] = aws.StringValue(creds.SecretAccessKey)
	terragruntOptions.Env["AWS_SESSION_TOKEN"] = aws.StringValue(creds.SessionToken)
	terragruntOptions.Env["AWS_SECURITY_TOKEN"] = aws.StringValue(creds.SessionToken)

	return nil
}
