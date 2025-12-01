// Package awshelper provides helper functions for working with AWS services.
package awshelper

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/gruntwork-io/go-commons/version"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// Minimum ARN parts required for a valid ARN
	minARNParts = 2
)

// AwsSessionConfig is a representation of the configuration options for an AWS Config
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

type tokenFetcher string

// FetchToken implements the token fetcher interface.
// Supports providing a token value or the path to a token on disk
func (f tokenFetcher) FetchToken(_ context.Context) ([]byte, error) {
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

func CreateS3Client(ctx context.Context, l log.Logger, config *AwsSessionConfig, opts *options.TerragruntOptions) (*s3.Client, error) {
	cfg, err := CreateAwsConfig(ctx, l, config, opts)
	if err != nil {
		return nil, errors.New(err)
	}

	var customFN []func(*s3.Options)
	if config.CustomS3Endpoint != "" {
		customFN = append(customFN, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(config.CustomS3Endpoint)
		})
	}

	if config.S3ForcePathStyle {
		customFN = append(customFN, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	return s3.NewFromConfig(cfg, customFN...), nil
}

// CreateAwsConfig returns an AWS config object for the given AwsSessionConfig and TerragruntOptions.
func CreateAwsConfig(
	ctx context.Context,
	l log.Logger,
	awsCfg *AwsSessionConfig,
	opts *options.TerragruntOptions,
) (aws.Config, error) {
	var configOptions []func(*config.LoadOptions) error

	configOptions = append(configOptions, config.WithAppID("terragrunt/"+version.GetVersion()))

	if envCreds := createCredentialsFromEnv(opts); envCreds != nil {
		l.Debugf("Using AWS credentials from auth provider command")

		configOptions = append(configOptions, config.WithCredentialsProvider(envCreds))
	} else if awsCfg != nil && awsCfg.CredsFilename != "" {
		configOptions = append(configOptions, config.WithSharedConfigFiles([]string{awsCfg.CredsFilename}))
	}

	// Prioritize configured region over environment variables
	// This fixes the issue where AWS_REGION/AWS_DEFAULT_REGION env vars override the backend config region
	var region string
	if awsCfg != nil && awsCfg.Region != "" {
		region = awsCfg.Region
	} else {
		region = getRegionFromEnv(opts)
	}

	if region == "" {
		region = "us-east-1"
	}

	configOptions = append(configOptions, config.WithRegion(region))

	if awsCfg != nil && awsCfg.Profile != "" {
		configOptions = append(configOptions, config.WithSharedConfigProfile(awsCfg.Profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, configOptions...)
	if err != nil {
		return aws.Config{}, errors.Errorf("Error loading AWS config: %w", err)
	}

	if createCredentialsFromEnv(opts) != nil {
		return cfg, nil
	}

	iamRoleOptions := getMergedIAMRoleOptions(awsCfg, opts)
	if iamRoleOptions.RoleARN == "" {
		return cfg, nil
	}

	if iamRoleOptions.WebIdentityToken != "" {
		l.Debugf("Assuming role %s using WebIdentity token", iamRoleOptions.RoleARN)
		cfg.Credentials = getWebIdentityCredentialsFromIAMRoleOptions(cfg, iamRoleOptions)

		return cfg, nil
	}

	l.Debugf("Assuming role %s", iamRoleOptions.RoleARN)
	cfg.Credentials = getSTSCredentialsFromIAMRoleOptions(cfg, iamRoleOptions, getExternalID(awsCfg))

	return cfg, nil
}

// getRegionFromEnv extracts region from environment variables in opts
func getRegionFromEnv(opts *options.TerragruntOptions) string {
	if opts == nil || opts.Env == nil {
		return ""
	}

	if region := opts.Env["AWS_REGION"]; region != "" {
		return region
	}

	return opts.Env["AWS_DEFAULT_REGION"]
}

// getMergedIAMRoleOptions merges IAM role options from awsCfg and opts
func getMergedIAMRoleOptions(awsCfg *AwsSessionConfig, opts *options.TerragruntOptions) options.IAMRoleOptions {
	iamRoleOptions := options.IAMRoleOptions{}

	// Start with opts IAM role options if available
	if opts != nil {
		iamRoleOptions = opts.IAMRoleOptions
	}

	// Merge in awsCfg role options if available
	if awsCfg != nil && awsCfg.RoleArn != "" {
		iamRoleOptions = options.MergeIAMRoleOptions(
			iamRoleOptions,
			options.IAMRoleOptions{
				RoleARN:               awsCfg.RoleArn,
				AssumeRoleSessionName: awsCfg.SessionName,
			},
		)
	}

	return iamRoleOptions
}

// getExternalID returns the external ID from awsCfg if available
func getExternalID(awsCfg *AwsSessionConfig) string {
	if awsCfg == nil {
		return ""
	}

	return awsCfg.ExternalID
}

// AssumeIamRole assumes an IAM role and returns the credentials
func AssumeIamRole(
	ctx context.Context,
	iamRoleOpts options.IAMRoleOptions,
	externalID string,
	opts *options.TerragruntOptions,
) (*types.Credentials, error) {
	region := getRegionFromEnv(opts)
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}

	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}

	if region == "" {
		region = "us-east-1"
	}

	// Set user agent to include terragrunt version
	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(region),
		config.WithAppID("terragrunt/"+version.GetVersion()),
	)
	if err != nil {
		return nil, errors.Errorf("Error loading AWS config: %w", err)
	}

	stsClient := sts.NewFromConfig(cfg)

	roleSessionName := iamRoleOpts.AssumeRoleSessionName
	if roleSessionName == "" {
		roleSessionName = options.GetDefaultIAMAssumeRoleSessionName()
	}

	duration := time.Duration(options.DefaultIAMAssumeRoleDuration) * time.Second
	if iamRoleOpts.AssumeRoleDuration > 0 {
		duration = time.Duration(iamRoleOpts.AssumeRoleDuration) * time.Second
	}

	if iamRoleOpts.WebIdentityToken != "" {
		// Use sts AssumeRoleWithWebIdentity
		tb, err := tokenFetcher(iamRoleOpts.WebIdentityToken).FetchToken(ctx)
		if err != nil {
			return nil, errors.Errorf("Error reading web identity token file: %w", err)
		}

		input := &sts.AssumeRoleWithWebIdentityInput{
			RoleArn:          aws.String(iamRoleOpts.RoleARN),
			RoleSessionName:  aws.String(roleSessionName),
			WebIdentityToken: aws.String(string(tb)),
			DurationSeconds:  aws.Int32(int32(duration.Seconds())),
		}

		result, err := stsClient.AssumeRoleWithWebIdentity(ctx, input)
		if err != nil {
			return nil, errors.Errorf("Error assuming role with web identity: %w", err)
		}

		return result.Credentials, nil
	}

	// Use regular sts AssumeRole
	input := &sts.AssumeRoleInput{
		RoleArn:         aws.String(iamRoleOpts.RoleARN),
		RoleSessionName: aws.String(roleSessionName),
		DurationSeconds: aws.Int32(int32(duration.Seconds())),
	}

	if externalID != "" {
		input.ExternalId = aws.String(externalID)
	}

	result, err := stsClient.AssumeRole(ctx, input)
	if err != nil {
		return nil, errors.Errorf("Error assuming role: %w", err)
	}

	return result.Credentials, nil
}

// GetAWSCallerIdentity gets the caller identity from AWS
func GetAWSCallerIdentity(ctx context.Context, cfg aws.Config) (*sts.GetCallerIdentityOutput, error) {
	stsClient := sts.NewFromConfig(cfg)
	return stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
}

// ValidateAwsConfig validates that the AWS config has valid credentials
func ValidateAwsConfig(ctx context.Context, cfg aws.Config) error {
	_, err := GetAWSCallerIdentity(ctx, cfg)
	return err
}

// GetAWSPartition gets the AWS partition from the caller identity
func GetAWSPartition(ctx context.Context, cfg aws.Config) (string, error) {
	result, err := GetAWSCallerIdentity(ctx, cfg)
	if err != nil {
		return "", err
	}

	// Extract partition from ARN
	arn := aws.ToString(result.Arn)
	if arn == "" {
		return "", errors.New("Empty ARN returned from GetCallerIdentity")
	}

	// ARN format: arn:partition:service:region:account:resource
	parts := strings.Split(arn, ":")
	if len(parts) < minARNParts {
		return "", errors.Errorf("Invalid ARN format: %s", arn)
	}

	return parts[1], nil
}

// GetAWSAccountAlias gets the AWS account alias
func GetAWSAccountAlias(ctx context.Context, cfg aws.Config) (string, error) {
	iamClient := iam.NewFromConfig(cfg)

	result, err := iamClient.ListAccountAliases(ctx, &iam.ListAccountAliasesInput{})
	if err != nil {
		return "", err
	}

	if len(result.AccountAliases) == 0 {
		return "", nil
	}

	return result.AccountAliases[0], nil
}

// GetAWSAccountID gets the AWS account ID from the caller identity
func GetAWSAccountID(ctx context.Context, cfg aws.Config) (string, error) {
	result, err := GetAWSCallerIdentity(ctx, cfg)
	if err != nil {
		return "", err
	}

	return aws.ToString(result.Account), nil
}

// GetAWSIdentityArn gets the AWS identity ARN from the caller identity
func GetAWSIdentityArn(ctx context.Context, cfg aws.Config) (string, error) {
	result, err := GetAWSCallerIdentity(ctx, cfg)
	if err != nil {
		return "", err
	}

	return aws.ToString(result.Arn), nil
}

// GetAWSUserID gets the AWS user ID from the caller identity
func GetAWSUserID(ctx context.Context, cfg aws.Config) (string, error) {
	result, err := GetAWSCallerIdentity(ctx, cfg)
	if err != nil {
		return "", err
	}

	return aws.ToString(result.UserId), nil
}

// ValidatePublicAccessBlock validates the public access block configuration
func ValidatePublicAccessBlock(output *s3.GetPublicAccessBlockOutput) (bool, error) {
	if output.PublicAccessBlockConfiguration == nil {
		return false, nil
	}

	config := output.PublicAccessBlockConfiguration

	return aws.ToBool(config.BlockPublicAcls) &&
		aws.ToBool(config.BlockPublicPolicy) &&
		aws.ToBool(config.IgnorePublicAcls) &&
		aws.ToBool(config.RestrictPublicBuckets), nil
}

func getWebIdentityCredentialsFromIAMRoleOptions(cfg aws.Config, iamRoleOptions options.IAMRoleOptions) aws.CredentialsProviderFunc {
	roleSessionName := iamRoleOptions.AssumeRoleSessionName
	if roleSessionName == "" {
		// Set a unique session name in the same way it is done in the SDK
		roleSessionName = strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}

	return func(ctx context.Context) (aws.Credentials, error) {
		stsClient := sts.NewFromConfig(cfg)

		token, err := tokenFetcher(iamRoleOptions.WebIdentityToken).FetchToken(ctx)
		if err != nil {
			return aws.Credentials{}, err
		}

		duration := time.Duration(options.DefaultIAMAssumeRoleDuration) * time.Second
		if iamRoleOptions.AssumeRoleDuration > 0 {
			duration = time.Duration(iamRoleOptions.AssumeRoleDuration) * time.Second
		}

		input := &sts.AssumeRoleWithWebIdentityInput{
			RoleArn:          aws.String(iamRoleOptions.RoleARN),
			RoleSessionName:  aws.String(roleSessionName),
			WebIdentityToken: aws.String(string(token)),
			DurationSeconds:  aws.Int32(int32(duration.Seconds())),
		}

		result, err := stsClient.AssumeRoleWithWebIdentity(ctx, input)
		if err != nil {
			return aws.Credentials{}, err
		}

		return aws.Credentials{
			AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
			SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
			SessionToken:    aws.ToString(result.Credentials.SessionToken),
			CanExpire:       true,
			Expires:         aws.ToTime(result.Credentials.Expiration),
		}, nil
	}
}

func getSTSCredentialsFromIAMRoleOptions(cfg aws.Config, iamRoleOptions options.IAMRoleOptions, externalID string) aws.CredentialsProviderFunc {
	return func(ctx context.Context) (aws.Credentials, error) {
		stsClient := sts.NewFromConfig(cfg)

		roleSessionName := iamRoleOptions.AssumeRoleSessionName
		if roleSessionName == "" {
			roleSessionName = strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
		}

		duration := time.Duration(options.DefaultIAMAssumeRoleDuration) * time.Second
		if iamRoleOptions.AssumeRoleDuration > 0 {
			duration = time.Duration(iamRoleOptions.AssumeRoleDuration) * time.Second
		}

		input := &sts.AssumeRoleInput{
			RoleArn:         aws.String(iamRoleOptions.RoleARN),
			RoleSessionName: aws.String(roleSessionName),
			DurationSeconds: aws.Int32(int32(duration.Seconds())),
		}

		if externalID != "" {
			input.ExternalId = aws.String(externalID)
		}

		result, err := stsClient.AssumeRole(ctx, input)
		if err != nil {
			return aws.Credentials{}, err
		}

		return aws.Credentials{
			AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
			SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
			SessionToken:    aws.ToString(result.Credentials.SessionToken),
			CanExpire:       true,
			Expires:         aws.ToTime(result.Credentials.Expiration),
		}, nil
	}
}

// createCredentialsFromEnv creates AWS credentials from environment variables in opts.Env
func createCredentialsFromEnv(opts *options.TerragruntOptions) aws.CredentialsProvider {
	if opts == nil || opts.Env == nil {
		return nil
	}

	accessKeyID := opts.Env["AWS_ACCESS_KEY_ID"]
	secretAccessKey := opts.Env["AWS_SECRET_ACCESS_KEY"]
	sessionToken := opts.Env["AWS_SESSION_TOKEN"]

	// If we don't have at least access key and secret key, return nil
	if accessKeyID == "" || secretAccessKey == "" {
		return nil
	}

	return credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, sessionToken)
}
