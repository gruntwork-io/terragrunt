// Package awshelper provides helper functions for working with AWS services.
package awshelper

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
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

// CreateAwsConfigFromConfig returns an AWS config object for the given config region (required), profile name (optional), and IAM role to assume
// (optional), ensuring that the credentials are available.
func CreateAwsConfigFromConfig(ctx context.Context, awsCfg *AwsSessionConfig, opts *options.TerragruntOptions) (aws.Config, error) {
	var configOptions []func(*config.LoadOptions) error

	// Set region
	if awsCfg.Region != "" {
		configOptions = append(configOptions, config.WithRegion(awsCfg.Region))
	}

	// Set profile
	if awsCfg.Profile != "" {
		configOptions = append(configOptions, config.WithSharedConfigProfile(awsCfg.Profile))
	}

	// Set custom credentials file
	if len(awsCfg.CredsFilename) > 0 {
		configOptions = append(configOptions, config.WithSharedConfigFiles([]string{awsCfg.CredsFilename}))
	}

	// Load default config
	cfg, err := config.LoadDefaultConfig(ctx, configOptions...)
	if err != nil {
		return aws.Config{}, errors.Errorf("Error loading AWS config: %w", err)
	}

	// Handle IAM role options
	iamRoleOptions := opts.IAMRoleOptions
	if awsCfg.RoleArn != "" {
		iamRoleOptions = options.MergeIAMRoleOptions(
			iamRoleOptions,
			options.IAMRoleOptions{
				RoleARN:               awsCfg.RoleArn,
				AssumeRoleSessionName: awsCfg.SessionName,
			},
		)
	}

	// Handle web identity credentials
	if iamRoleOptions.WebIdentityToken != "" && iamRoleOptions.RoleARN != "" {
		cfg.Credentials = getWebIdentityCredentialsFromIAMRoleOptions(cfg, iamRoleOptions)
		return cfg, nil
	}

	// Handle STS role assumption
	if iamRoleOptions.RoleARN != "" {
		cfg.Credentials = getSTSCredentialsFromIAMRoleOptions(cfg, iamRoleOptions, awsCfg.ExternalID)
	} else if creds := getCredentialsFromEnvs(opts); creds != nil {
		cfg.Credentials = creds
	}

	return cfg, nil
}

type tokenFetcher string

// FetchToken implements the token fetcher interface.
// Supports providing a token value or the path to a token on disk
func (f tokenFetcher) FetchToken(ctx context.Context) ([]byte, error) {
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

func getWebIdentityCredentialsFromIAMRoleOptions(cfg aws.Config, iamRoleOptions options.IAMRoleOptions) aws.CredentialsProviderFunc {
	roleSessionName := iamRoleOptions.AssumeRoleSessionName
	if roleSessionName == "" {
		// Set a unique session name in the same way it is done in the SDK
		roleSessionName = strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}

	return aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
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
	})
}

func getSTSCredentialsFromIAMRoleOptions(cfg aws.Config, iamRoleOptions options.IAMRoleOptions, externalID string) aws.CredentialsProviderFunc {
	return aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
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
	})
}

func getCredentialsFromEnvs(opts *options.TerragruntOptions) aws.CredentialsProviderFunc {
	var (
		accessKeyID     = opts.Env["AWS_ACCESS_KEY_ID"]
		secretAccessKey = opts.Env["AWS_SECRET_ACCESS_KEY"]
		sessionToken    = opts.Env["AWS_SESSION_TOKEN"]
	)

	if accessKeyID == "" || secretAccessKey == "" {
		return nil
	}

	return aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		return aws.Credentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			SessionToken:    sessionToken,
		}, nil
	})
}

func CreateS3Client(ctx context.Context, l log.Logger, config *AwsSessionConfig, opts *options.TerragruntOptions) (*s3.Client, error) {
	cfg, err := CreateAwsConfig(ctx, l, config, opts)
	if err != nil {
		return nil, errors.New(err)
	}

	return s3.NewFromConfig(cfg), nil
}

// CreateAwsConfig returns an AWS config object for the given config region (required), profile name (optional), and IAM role to assume
// (optional), ensuring that the credentials are available.
func CreateAwsConfig(ctx context.Context, l log.Logger, awsCfg *AwsSessionConfig, opts *options.TerragruntOptions) (aws.Config, error) {
	var cfg aws.Config

	var err error

	if awsCfg == nil {
		cfg, err = config.LoadDefaultConfig(ctx)
		if err != nil {
			return aws.Config{}, errors.New(err)
		}

		// Ensure a region is set - fallback to us-east-1 if none is configured
		if cfg.Region == "" {
			l.Debugf("No region configured, using default region us-east-1")

			cfg.Region = "us-east-1"
		}

		if opts.IAMRoleOptions.RoleARN != "" {
			if opts.IAMRoleOptions.WebIdentityToken != "" {
				l.Debugf("Assuming role %s using WebIdentity token", opts.IAMRoleOptions.RoleARN)
				cfg.Credentials = getWebIdentityCredentialsFromIAMRoleOptions(cfg, opts.IAMRoleOptions)
			} else {
				l.Debugf("Assuming role %s", opts.IAMRoleOptions.RoleARN)
				cfg.Credentials = getSTSCredentialsFromIAMRoleOptions(cfg, opts.IAMRoleOptions, "")
			}
		} else if creds := getCredentialsFromEnvs(opts); creds != nil {
			cfg.Credentials = creds
		}
	} else {
		cfg, err = CreateAwsConfigFromConfig(ctx, awsCfg, opts)
		if err != nil {
			return aws.Config{}, errors.New(err)
		}
	}

	// Validate credentials
	if err = ValidateAwsConfig(ctx, cfg); err != nil {
		// construct dynamic error message based on the configuration
		msg := "Error finding AWS credentials (did you set the AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables?)"
		if awsCfg != nil && len(awsCfg.CredsFilename) > 0 {
			msg = fmt.Sprintf("Error finding AWS credentials in file '%s' (did you set the correct file name and/or profile?)", awsCfg.CredsFilename)
		}

		return aws.Config{}, errors.Errorf("%s: %w", msg, err)
	}

	return cfg, nil
}

// AssumeIamRole assumes an IAM role and returns the credentials
func AssumeIamRole(ctx context.Context, iamRoleOpts options.IAMRoleOptions, externalID string) (*types.Credentials, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
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

	if iamRoleOpts.WebIdentityToken == "" {
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
			return nil, errors.New(err)
		}

		return result.Credentials, nil
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

	input := &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(iamRoleOpts.RoleARN),
		RoleSessionName:  aws.String(roleSessionName),
		WebIdentityToken: aws.String(token),
		DurationSeconds:  aws.Int32(int32(duration.Seconds())),
	}

	result, err := stsClient.AssumeRoleWithWebIdentity(ctx, input)
	if err != nil {
		return nil, errors.New(err)
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
