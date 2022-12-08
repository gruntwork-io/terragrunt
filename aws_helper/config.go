package aws_helper

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/credentials"
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

	// Merge the config based IAMRole options into the original one, as the config has higher precedence than CLI.
	iamRoleOptions := terragruntOptions.IAMRoleOptions
	if config.RoleArn != "" {
		iamRoleOptions = options.MergeIAMRoleOptions(
			iamRoleOptions,
			options.IAMRoleOptions{
				RoleARN:               config.RoleArn,
				AssumeRoleSessionName: config.SessionName,
			},
		)
	}

	credentialOptFn := func(p *stscreds.AssumeRoleProvider) {
		if config.ExternalID != "" {
			p.ExternalID = aws.String(config.ExternalID)
		}
	}

	if iamRoleOptions.RoleARN != "" {
		sess.Config.Credentials = getSTSCredentialsFromIAMRoleOptions(sess, iamRoleOptions, credentialOptFn)
	}
	return sess, nil
}

func getSTSCredentialsFromIAMRoleOptions(sess *session.Session, iamRoleOptions options.IAMRoleOptions, optFns ...func(*stscreds.AssumeRoleProvider)) *credentials.Credentials {
	optFns = append(optFns, func(p *stscreds.AssumeRoleProvider) {
		if iamRoleOptions.AssumeRoleDuration > 0 {
			p.Duration = time.Second * time.Duration(iamRoleOptions.AssumeRoleDuration)
		} else {
			p.Duration = time.Second * time.Duration(options.DefaultIAMAssumeRoleDuration)
		}
		if iamRoleOptions.AssumeRoleSessionName != "" {
			p.RoleSessionName = iamRoleOptions.AssumeRoleSessionName
		}
	})
	return stscreds.NewCredentials(sess, iamRoleOptions.RoleARN, optFns...)
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
		sessionOptions := session.Options{SharedConfigState: session.SharedConfigEnable}
		sess, err = session.NewSessionWithOptions(sessionOptions)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		if terragruntOptions.IAMRoleOptions.RoleARN != "" {
			terragruntOptions.Logger.Debugf("Assuming role %s", terragruntOptions.IAMRoleOptions.RoleARN)
			sess.Config.Credentials = getSTSCredentialsFromIAMRoleOptions(sess, terragruntOptions.IAMRoleOptions)
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
func AssumeIamRole(iamRoleOpts options.IAMRoleOptions) (*sts.Credentials, error) {
	sessionOptions := session.Options{SharedConfigState: session.SharedConfigEnable}
	sess, err := session.NewSessionWithOptions(sessionOptions)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	_, err = sess.Config.Credentials.Get()
	if err != nil {
		return nil, errors.WithStackTraceAndPrefix(err, "Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?)")
	}

	stsClient := sts.New(sess)

	sessionName := options.GetDefaultIAMAssumeRoleSessionName()
	if iamRoleOpts.AssumeRoleSessionName != "" {
		sessionName = iamRoleOpts.AssumeRoleSessionName
	}

	sessionDurationSeconds := int64(options.DefaultIAMAssumeRoleDuration)
	if iamRoleOpts.AssumeRoleDuration != 0 {
		sessionDurationSeconds = iamRoleOpts.AssumeRoleDuration
	}

	input := sts.AssumeRoleInput{
		RoleArn:         aws.String(iamRoleOpts.RoleARN),
		RoleSessionName: aws.String(sessionName),
		DurationSeconds: aws.Int64(sessionDurationSeconds),
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

// Get the AWS Partition of the current session configuration
func GetAWSPartition(config *AwsSessionConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	identity, err := GetAWSCallerIdentity(config, terragruntOptions)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	arn, err := arn.Parse(*identity.Arn)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	return arn.Partition, nil
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
	iamRoleOpts := terragruntOptions.IAMRoleOptions
	if iamRoleOpts.RoleARN == "" {
		return nil
	}

	terragruntOptions.Logger.Debugf("Assuming IAM role %s with a session duration of %d seconds.", iamRoleOpts.RoleARN, iamRoleOpts.AssumeRoleDuration)
	creds, err := AssumeIamRole(iamRoleOpts)
	if err != nil {
		return err
	}

	terragruntOptions.Env["AWS_ACCESS_KEY_ID"] = aws.StringValue(creds.AccessKeyId)
	terragruntOptions.Env["AWS_SECRET_ACCESS_KEY"] = aws.StringValue(creds.SecretAccessKey)
	terragruntOptions.Env["AWS_SESSION_TOKEN"] = aws.StringValue(creds.SessionToken)
	terragruntOptions.Env["AWS_SECURITY_TOKEN"] = aws.StringValue(creds.SessionToken)

	return nil
}
