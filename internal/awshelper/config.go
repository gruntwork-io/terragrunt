// Package awshelper provides helper functions for working with AWS services.
package awshelper

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gruntwork-io/go-commons/version"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

// AwsSessionConfig is a representation of the configuration options for an AWS Session
type AwsSessionConfig struct {
	Tags                    map[string]string
	Region                  string
	CustomS3Endpoint        string
	CustomDynamoDBEndpoint  string
	Profile                 string
	RoleArn                 string
	CredsFilename           string
	ExternalID              string
	SessionName             string
	S3ForcePathStyle        bool
	DisableComputeChecksums bool
}

// addUserAgent - Add terragrunt version to the user agent for AWS API calls.
var addUserAgent = request.NamedHandler{
	Name: "terragrunt.UserAgentHandler",
	Fn: request.MakeAddToUserAgentHandler(
		"terragrunt", version.GetVersion()),
}

// CreateAwsSessionFromConfig returns an AWS session object for the given config region (required), profile name (optional), and IAM role to assume
// (optional), ensuring that the credentials are available.
func CreateAwsSessionFromConfig(config *AwsSessionConfig, opts *options.TerragruntOptions) (*session.Session, error) {
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
		return nil, errors.Errorf("Error initializing session: %w", err)
	}

	sess.Handlers.Build.PushFrontNamed(addUserAgent)

	// Merge the config based IAMRole options into the original one, as the config has higher precedence than CLI.
	iamRoleOptions := opts.IAMRoleOptions
	if config.RoleArn != "" {
		iamRoleOptions = options.MergeIAMRoleOptions(
			iamRoleOptions,
			options.IAMRoleOptions{
				RoleARN:               config.RoleArn,
				AssumeRoleSessionName: config.SessionName,
			},
		)
	}

	if iamRoleOptions.WebIdentityToken != "" && iamRoleOptions.RoleARN != "" {
		sess.Config.Credentials = getWebIdentityCredentialsFromIAMRoleOptions(sess, iamRoleOptions)
		return sess, nil
	}

	credentialOptFn := func(p *stscreds.AssumeRoleProvider) {
		if config.ExternalID != "" {
			p.ExternalID = aws.String(config.ExternalID)
		}
	}

	if iamRoleOptions.RoleARN != "" {
		sess.Config.Credentials = getSTSCredentialsFromIAMRoleOptions(sess, iamRoleOptions, credentialOptFn)
	} else if creds := getCredentialsFromEnvs(opts); creds != nil {
		sess.Config.Credentials = creds
	}

	return sess, nil
}

type tokenFetcher string

// FetchToken Implements the stscreds.TokenFetcher interface.
// Supports providing a token value or the path to a token on disk
func (f tokenFetcher) FetchToken(ctx credentials.Context) ([]byte, error) {
	// Check if token is a raw value
	if _, err := os.Stat(string(f)); err != nil {
		// TODO: See if this lint error should be ignored
		return []byte(f), nil //nolint: nilerr
	}

	token, err := os.ReadFile(string(f))
	if err != nil {
		return nil, errors.New(err)
	}

	return token, nil
}

func getWebIdentityCredentialsFromIAMRoleOptions(sess *session.Session, iamRoleOptions options.IAMRoleOptions) *credentials.Credentials {
	roleSessionName := iamRoleOptions.AssumeRoleSessionName
	if roleSessionName == "" {
		// Set a unique session name in the same way it is done in the SDK
		roleSessionName = strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}

	svc := sts.New(sess)
	p := stscreds.NewWebIdentityRoleProviderWithOptions(svc, iamRoleOptions.RoleARN, roleSessionName, tokenFetcher(iamRoleOptions.WebIdentityToken))

	if iamRoleOptions.AssumeRoleDuration > 0 {
		p.Duration = time.Second * time.Duration(iamRoleOptions.AssumeRoleDuration)
	} else {
		p.Duration = time.Second * time.Duration(options.DefaultIAMAssumeRoleDuration)
	}

	return credentials.NewCredentials(p)
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

func getCredentialsFromEnvs(opts *options.TerragruntOptions) *credentials.Credentials {
	var (
		accessKeyID     = opts.Env["AWS_ACCESS_KEY_ID"]
		secretAccessKey = opts.Env["AWS_SECRET_ACCESS_KEY"]
		sessionToken    = opts.Env["AWS_SESSION_TOKEN"]
	)

	if accessKeyID == "" || secretAccessKey == "" {
		return nil
	}

	return credentials.NewStaticCredentials(accessKeyID, secretAccessKey, sessionToken)
}

func CreateS3Client(l log.Logger, config *AwsSessionConfig, opts *options.TerragruntOptions) (*s3.S3, error) {
	session, err := CreateAwsSession(l, config, opts)
	if err != nil {
		return nil, errors.New(err)
	}

	return s3.New(session), nil
}

// CreateAwsSession returns an AWS session object. The session is configured by either:
//   - The provided AwsSessionConfig struct, which specifies region (required), profile name (optional), and IAM role to
//     assume (optional).
//   - The provided TerragruntOptions struct, which specifies any IAM role to assume (optional).
//
// Note that if the AwsSessionConfig object is null, this will return default session credentials using the default
// credentials chain of the AWS SDK.
func CreateAwsSession(l log.Logger, config *AwsSessionConfig, opts *options.TerragruntOptions) (*session.Session, error) {
	var (
		sess *session.Session
		err  error
	)

	if config == nil {
		sessionOptions := session.Options{SharedConfigState: session.SharedConfigEnable}

		sess, err = session.NewSessionWithOptions(sessionOptions)
		if err != nil {
			return nil, errors.New(err)
		}

		sess.Handlers.Build.PushFrontNamed(addUserAgent)

		if opts.IAMRoleOptions.RoleARN != "" {
			if opts.IAMRoleOptions.WebIdentityToken != "" {
				l.Debugf("Assuming role %s using WebIdentity token", opts.IAMRoleOptions.RoleARN)
				sess.Config.Credentials = getWebIdentityCredentialsFromIAMRoleOptions(sess, opts.IAMRoleOptions)
			} else {
				l.Debugf("Assuming role %s", opts.IAMRoleOptions.RoleARN)
				sess.Config.Credentials = getSTSCredentialsFromIAMRoleOptions(sess, opts.IAMRoleOptions)
			}
		} else if creds := getCredentialsFromEnvs(opts); creds != nil {
			sess.Config.Credentials = creds
		}
	} else {
		sess, err = CreateAwsSessionFromConfig(config, opts)
		if err != nil {
			return nil, errors.New(err)
		}
	}

	if _, err = sess.Config.Credentials.Get(); err != nil {
		// construct dynamic error message based on the configuration
		msg := "Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?)"
		if config != nil && len(config.CredsFilename) > 0 {
			msg = fmt.Sprintf("Error finding AWS credentials in file '%s' (did you set the correct file name and/or profile?)", config.CredsFilename)
		}

		return nil, errors.Errorf("%s: %w", msg, err)
	}

	return sess, nil
}

// AssumeIamRole makes API calls to AWS to assume the IAM role specified and return the temporary AWS credentials to use that role.
func AssumeIamRole(iamRoleOpts options.IAMRoleOptions) (*sts.Credentials, error) {
	sessionOptions := session.Options{SharedConfigState: session.SharedConfigEnable}

	sess, err := session.NewSessionWithOptions(sessionOptions)
	if err != nil {
		return nil, errors.New(err)
	}

	sess.Handlers.Build.PushFrontNamed(addUserAgent)

	if iamRoleOpts.RoleARN != "" && iamRoleOpts.WebIdentityToken != "" {
		sess.Config.Credentials = getWebIdentityCredentialsFromIAMRoleOptions(sess, iamRoleOpts)
	}

	_, err = sess.Config.Credentials.Get()
	if err != nil {
		return nil, errors.Errorf("error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?): %w", err)
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

	if iamRoleOpts.WebIdentityToken == "" {
		// Use regular sts AssumeRole
		input := sts.AssumeRoleInput{
			RoleArn:         aws.String(iamRoleOpts.RoleARN),
			RoleSessionName: aws.String(sessionName),
			DurationSeconds: aws.Int64(sessionDurationSeconds),
		}

		output, err := stsClient.AssumeRole(&input)
		if err != nil {
			return nil, errors.New(err)
		}

		return output.Credentials, nil
	}

	// Use sts AssumeRoleWithWebIdentity
	var token string
	// Check if value is a raw token or a path to a file with a token
	if _, err := os.Stat(iamRoleOpts.WebIdentityToken); err != nil {
		token = iamRoleOpts.WebIdentityToken
	} else {
		tb, err := os.ReadFile(iamRoleOpts.WebIdentityToken)
		if err != nil {
			return nil, errors.New(err)
		}

		token = string(tb)
	}

	input := sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(iamRoleOpts.RoleARN),
		RoleSessionName:  aws.String(sessionName),
		WebIdentityToken: aws.String(token),
		DurationSeconds:  aws.Int64(sessionDurationSeconds),
	}
	req, resp := stsClient.AssumeRoleWithWebIdentityRequest(&input)
	// InvalidIdentityToken error is a temporary error that can occur
	// when assuming an Role with a JWT web identity token.
	// N.B: copied from SDK implementation
	req.RetryErrorCodes = append(req.RetryErrorCodes, sts.ErrCodeInvalidIdentityTokenException)
	if err := req.Send(); err != nil {
		return nil, errors.New(err)
	}

	return resp.Credentials, nil
}

// GetAWSCallerIdentity returns the AWS caller identity associated with the current set of credentials
func GetAWSCallerIdentity(sess *session.Session) (sts.GetCallerIdentityOutput, error) {
	identity, err := sts.New(sess).GetCallerIdentity(nil)
	if err != nil {
		return sts.GetCallerIdentityOutput{}, errors.New(err)
	}

	return *identity, nil
}

// ValidateAwsSession - Validate if current AWS session is valid
func ValidateAwsSession(sess *session.Session) error {
	// read the caller identity to check if the credentials are valid
	_, err := GetAWSCallerIdentity(sess)
	return err
}

// GetAWSPartition gets the AWS Partition of the current session configuration
func GetAWSPartition(sess *session.Session) (string, error) {
	identity, err := GetAWSCallerIdentity(sess)
	if err != nil {
		return "", errors.New(err)
	}

	arn, err := arn.Parse(*identity.Arn)
	if err != nil {
		return "", errors.New(err)
	}

	return arn.Partition, nil
}

// GetAWSAccountAlias gets the AWS account Alias of the current session configuration,
// if there is no alias an empty string is return.
func GetAWSAccountAlias(sess *session.Session) (string, error) {
	aliases, err := iam.New(sess).ListAccountAliases(nil)
	if err != nil {
		return "", errors.New(err)
	}

	if len(aliases.AccountAliases) != 1 { // AWS supports only one alias per account https://docs.aws.amazon.com/IAM/latest/APIReference/API_ListAccountAliases.html
		return "", nil
	}

	alias := aliases.AccountAliases[0]
	if alias == nil {
		return "", errors.Errorf("expected AWS account alias, got nil")
	}

	return *alias, nil
}

// GetAWSAccountID gets the AWS account ID of the current session configuration.
func GetAWSAccountID(sess *session.Session) (string, error) {
	identity, err := GetAWSCallerIdentity(sess)
	if err != nil {
		return "", errors.New(err)
	}

	return *identity.Account, nil
}

// GetAWSIdentityArn gets the ARN of the AWS identity associated with the current set of credentials.
func GetAWSIdentityArn(sess *session.Session) (string, error) {
	identity, err := GetAWSCallerIdentity(sess)
	if err != nil {
		return "", errors.New(err)
	}

	return *identity.Arn, nil
}

// GetAWSUserID gets the AWS user ID of the current session configuration.
func GetAWSUserID(sess *session.Session) (string, error) {
	identity, err := GetAWSCallerIdentity(sess)
	if err != nil {
		return "", errors.New(err)
	}

	return *identity.UserId, nil
}

func ValidatePublicAccessBlock(output *s3.GetPublicAccessBlockOutput) (bool, error) {
	if output.PublicAccessBlockConfiguration == nil {
		return false, nil
	}

	if !aws.BoolValue(output.PublicAccessBlockConfiguration.BlockPublicAcls) {
		return false, nil
	}

	return true, nil
}
