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
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/gruntwork-io/go-commons/version"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
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

// AWSConfigBuilder builds an AWS config using the builder pattern.
// Use NewAwsConfigBuilder to create, chain With* methods for optional parameters, then call Build().
type AWSConfigBuilder struct {
	sessionConfig *AwsSessionConfig
	env           map[string]string
	iamRoleOpts   iam.RoleOptions
}

// NewAWSConfigBuilder creates a new builder for AWS config.
func NewAWSConfigBuilder() *AWSConfigBuilder {
	return &AWSConfigBuilder{
		env: make(map[string]string),
	}
}

// WithSessionConfig sets the AWS session configuration (region, profile, credentials file, etc.).
func (b *AWSConfigBuilder) WithSessionConfig(cfg *AwsSessionConfig) *AWSConfigBuilder {
	b.sessionConfig = cfg
	return b
}

// WithEnv sets environment variables used for credential and region resolution.
func (b *AWSConfigBuilder) WithEnv(env map[string]string) *AWSConfigBuilder {
	b.env = env
	return b
}

// WithIAMRoleOptions sets IAM role options for assuming a role.
func (b *AWSConfigBuilder) WithIAMRoleOptions(opts iam.RoleOptions) *AWSConfigBuilder {
	b.iamRoleOpts = opts
	return b
}

// Build creates the AWS config from the builder's configuration.
func (b *AWSConfigBuilder) Build(ctx context.Context, l log.Logger) (aws.Config, error) {
	var configOptions []func(*config.LoadOptions) error

	configOptions = append(configOptions, config.WithAppID("terragrunt/"+version.GetVersion()))

	if envCreds := createCredentialsFromEnv(b.env); envCreds != nil {
		l.Debugf("Using AWS credentials from auth provider command")

		configOptions = append(configOptions, config.WithCredentialsProvider(envCreds))
	} else if b.sessionConfig != nil && b.sessionConfig.CredsFilename != "" {
		configOptions = append(configOptions, config.WithSharedConfigFiles([]string{b.sessionConfig.CredsFilename}))
	}

	// Prioritize configured region over environment variables
	// This fixes the issue where AWS_REGION/AWS_DEFAULT_REGION env vars override the backend config region
	var region string
	if b.sessionConfig != nil && b.sessionConfig.Region != "" {
		region = b.sessionConfig.Region
	} else {
		region = getRegionFromEnv(b.env)
	}

	if region == "" {
		region = "us-east-1"
	}

	configOptions = append(configOptions, config.WithRegion(region))

	if b.sessionConfig != nil && b.sessionConfig.Profile != "" {
		configOptions = append(configOptions, config.WithSharedConfigProfile(b.sessionConfig.Profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, configOptions...)
	if err != nil {
		return aws.Config{}, errors.Errorf("Error loading AWS config: %w", err)
	}

	if createCredentialsFromEnv(b.env) != nil {
		return cfg, nil
	}

	mergedIAMRoleOptions := getMergedIAMRoleOptions(b.sessionConfig, b.iamRoleOpts)
	if mergedIAMRoleOptions.RoleARN == "" {
		return cfg, nil
	}

	if mergedIAMRoleOptions.WebIdentityToken != "" {
		l.Debugf("Assuming role %s using WebIdentity token", mergedIAMRoleOptions.RoleARN)
		cfg.Credentials = getWebIdentityCredentialsFromIAMRoleOptions(cfg, mergedIAMRoleOptions)

		return cfg, nil
	}

	l.Debugf("Assuming role %s", mergedIAMRoleOptions.RoleARN)
	cfg.Credentials = getSTSCredentialsFromIAMRoleOptions(cfg, mergedIAMRoleOptions, getExternalID(b.sessionConfig))

	return cfg, nil
}

// BuildS3Client creates an S3 client from the builder's configuration.
// The session config (set via WithSessionConfig) provides S3-specific options like custom endpoint and path style.
func (b *AWSConfigBuilder) BuildS3Client(ctx context.Context, l log.Logger) (*s3.Client, error) {
	cfg, err := b.Build(ctx, l)
	if err != nil {
		return nil, errors.New(err)
	}

	if b.sessionConfig == nil {
		return s3.NewFromConfig(cfg), nil
	}

	customFN := make([]func(*s3.Options), 0, 2) //nolint:mnd

	if b.sessionConfig.CustomS3Endpoint != "" {
		customFN = append(customFN, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(b.sessionConfig.CustomS3Endpoint)
		})
	}

	if b.sessionConfig.S3ForcePathStyle {
		customFN = append(customFN, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	return s3.NewFromConfig(cfg, customFN...), nil
}

// getRegionFromEnv extracts region from environment variables.
func getRegionFromEnv(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}

	if region := env["AWS_REGION"]; region != "" {
		return region
	}

	return env["AWS_DEFAULT_REGION"]
}

// getMergedIAMRoleOptions merges IAM role options from awsCfg and the provided IAM role options.
func getMergedIAMRoleOptions(awsCfg *AwsSessionConfig, iamRoleOpts iam.RoleOptions) iam.RoleOptions {
	// Merge in awsCfg role options if available
	if awsCfg != nil && awsCfg.RoleArn != "" {
		iamRoleOpts = iam.MergeRoleOptions(
			iamRoleOpts,
			iam.RoleOptions{
				RoleARN:               awsCfg.RoleArn,
				AssumeRoleSessionName: awsCfg.SessionName,
			},
		)
	}

	return iamRoleOpts
}

// getExternalID returns the external ID from awsCfg if available
func getExternalID(awsCfg *AwsSessionConfig) string {
	if awsCfg == nil {
		return ""
	}

	return awsCfg.ExternalID
}

// AssumeIamRole assumes an IAM role and returns the credentials.
func AssumeIamRole(
	ctx context.Context,
	iamRoleOpts iam.RoleOptions,
	externalID string,
	env map[string]string,
) (*types.Credentials, error) {
	region := getRegionFromEnv(env)
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
func GetAWSCallerIdentity(ctx context.Context, cfg *aws.Config) (*sts.GetCallerIdentityOutput, error) {
	stsClient := sts.NewFromConfig(*cfg)
	return stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
}

// ValidateAwsConfig validates that the AWS config has valid credentials
func ValidateAwsConfig(ctx context.Context, cfg *aws.Config) error {
	_, err := GetAWSCallerIdentity(ctx, cfg)
	return err
}

// GetAWSPartition gets the AWS partition from the caller identity
func GetAWSPartition(ctx context.Context, cfg *aws.Config) (string, error) {
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
func GetAWSAccountAlias(ctx context.Context, cfg *aws.Config) (string, error) {
	iamClient := awsiam.NewFromConfig(*cfg)

	result, err := iamClient.ListAccountAliases(ctx, &awsiam.ListAccountAliasesInput{})
	if err != nil {
		return "", err
	}

	if len(result.AccountAliases) == 0 {
		return "", nil
	}

	return result.AccountAliases[0], nil
}

// GetAWSAccountID gets the AWS account ID from the caller identity
func GetAWSAccountID(ctx context.Context, cfg *aws.Config) (string, error) {
	result, err := GetAWSCallerIdentity(ctx, cfg)
	if err != nil {
		return "", err
	}

	return aws.ToString(result.Account), nil
}

// GetAWSIdentityArn gets the AWS identity ARN from the caller identity
func GetAWSIdentityArn(ctx context.Context, cfg *aws.Config) (string, error) {
	result, err := GetAWSCallerIdentity(ctx, cfg)
	if err != nil {
		return "", err
	}

	return aws.ToString(result.Arn), nil
}

// GetAWSUserID gets the AWS user ID from the caller identity
func GetAWSUserID(ctx context.Context, cfg *aws.Config) (string, error) {
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

//nolint:gocritic // hugeParam: intentionally pass by value to avoid recursive credential resolution
func getWebIdentityCredentialsFromIAMRoleOptions(cfg aws.Config, iamRoleOptions iam.RoleOptions) aws.CredentialsProviderFunc {
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

//nolint:gocritic // hugeParam: intentionally pass by value to avoid recursive credential resolution
func getSTSCredentialsFromIAMRoleOptions(cfg aws.Config, iamRoleOptions iam.RoleOptions, externalID string) aws.CredentialsProviderFunc {
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

// createCredentialsFromEnv creates AWS credentials from environment variables.
func createCredentialsFromEnv(env map[string]string) aws.CredentialsProvider {
	if len(env) == 0 {
		return nil
	}

	accessKeyID := env["AWS_ACCESS_KEY_ID"]
	secretAccessKey := env["AWS_SECRET_ACCESS_KEY"]
	sessionToken := env["AWS_SESSION_TOKEN"]

	// If we don't have at least access key and secret key, return nil
	if accessKeyID == "" || secretAccessKey == "" {
		return nil
	}

	return credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, sessionToken)
}
