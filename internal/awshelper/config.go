// Package awshelper provides helper functions for working with AWS services.
package awshelper

import (
	"context"
	"encoding/json"
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
	"github.com/gruntwork-io/terragrunt/shell"
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

	region := getRegionFromEnv(opts)
	if region == "" && awsCfg != nil && awsCfg.Region != "" {
		region = awsCfg.Region
	}

	if region == "" {
		region = "us-east-1"
	}

	configOptions = append(configOptions, config.WithRegion(region))

	// Derive credentials/role from opts; if none, proactively run auth-provider-cmd to populate opts before config load
	envCreds := createCredentialsFromEnv(opts)
	role := options.MergeIAMRoleOptions(getMergedIAMRoleOptions(awsCfg, opts), getIAMRoleOptionsFromEnv(opts))

	if envCreds == nil && role.RoleARN == "" && opts != nil && opts.AuthProviderCmd != "" {
		if err := runAuthProviderCmdIntoOpts(ctx, l, opts); err == nil {
			// refresh
			envCreds = createCredentialsFromEnv(opts)
			role = options.MergeIAMRoleOptions(getMergedIAMRoleOptions(awsCfg, opts), getIAMRoleOptionsFromEnv(opts))
		}
	}

	if envCreds != nil {
		l.Debugf("Using AWS credentials from auth provider command")
		configOptions = append(configOptions, config.WithCredentialsProvider(envCreds))
	} else if role.RoleARN != "" && role.WebIdentityToken != "" {
		l.Debugf("Configuring web identity assume-role provider for %s", role.RoleARN)
		configOptions = append(configOptions, config.WithCredentialsProvider(newWebIdentityProvider(region, role)))
	}

	if awsCfg != nil && awsCfg.CredsFilename != "" {
		configOptions = append(configOptions, config.WithSharedConfigFiles([]string{awsCfg.CredsFilename}))
	}

	if awsCfg != nil && awsCfg.Profile != "" {
		configOptions = append(configOptions, config.WithSharedConfigProfile(awsCfg.Profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, configOptions...)
	if err != nil {
		return aws.Config{}, errors.Errorf("Error loading AWS config: %w", err)
	}

	// If still no provider on cfg, set explicitly from env creds or role
	if _, err := cfg.Credentials.Retrieve(ctx); err != nil {
		if envCreds != nil {
			cfg.Credentials = envCreds
		} else if role.RoleARN != "" && role.WebIdentityToken != "" {
			cfg.Credentials = getWebIdentityCredentialsFromIAMRoleOptions(cfg, role)
		}
	}

	return cfg, nil
}

// getRegionFromEnv extracts region from environment variables in opts
func getRegionFromEnv(opts *options.TerragruntOptions) string {
	if opts != nil && opts.Env != nil {
		if region := opts.Env["AWS_REGION"]; region != "" {
			return region
		}

		if region := opts.Env["AWS_DEFAULT_REGION"]; region != "" {
			return region
		}
	}

	return ""
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
	if awsCfg != nil {
		return awsCfg.ExternalID
	}

	return ""
}

// getIAMRoleOptionsFromEnv extracts IAM role options from opts.Env if present.
// Recognizes variables commonly used for web identity role assumption:
// - AWS_ROLE_ARN
// - AWS_WEB_IDENTITY_TOKEN_FILE or AWS_WEB_IDENTITY_TOKEN (raw token)
// - AWS_ROLE_SESSION_NAME (optional)
// - AWS_ROLE_DURATION or AWS_ROLE_DURATION_SECONDS (optional)
func getIAMRoleOptionsFromEnv(opts *options.TerragruntOptions) options.IAMRoleOptions {
	var iamRole options.IAMRoleOptions

	if opts == nil || opts.Env == nil {
		return iamRole
	}

	roleARN := opts.Env["AWS_ROLE_ARN"]
	if roleARN == "" {
		return iamRole
	}

	iamRole.RoleARN = roleARN

	if tokenFile := opts.Env["AWS_WEB_IDENTITY_TOKEN_FILE"]; tokenFile != "" {
		iamRole.WebIdentityToken = tokenFile
	} else if token := opts.Env["AWS_WEB_IDENTITY_TOKEN"]; token != "" {
		iamRole.WebIdentityToken = token
	}

	if session := opts.Env["AWS_ROLE_SESSION_NAME"]; session != "" {
		iamRole.AssumeRoleSessionName = session
	}

	if dur := opts.Env["AWS_ROLE_DURATION"]; dur != "" {
		if v, err := strconv.ParseInt(dur, 10, 64); err == nil {
			iamRole.AssumeRoleDuration = v
		}
	}

	if dur := opts.Env["AWS_ROLE_DURATION_SECONDS"]; dur != "" && iamRole.AssumeRoleDuration == 0 {
		if v, err := strconv.ParseInt(dur, 10, 64); err == nil {
			iamRole.AssumeRoleDuration = v
		}
	}

	return iamRole
}

// runAuthProviderCmdIntoOpts executes opts.AuthProviderCmd and merges returned credentials/envs into opts.
// It supports the same JSON schema documented for auth-provider-cmd.
func runAuthProviderCmdIntoOpts(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	cmd := strings.TrimSpace(opts.AuthProviderCmd)
	if cmd == "" {
		return nil
	}

	var (
		command = cmd
		args    []string
	)

	if parts := strings.Fields(cmd); len(parts) > 1 {
		command = parts[0]
		args = parts[1:]
	}

	out, err := shell.RunCommandWithOutput(ctx, l, opts, "", true, false, command, args...)
	if err != nil {
		return err
	}

	stdout := strings.TrimSpace(out.Stdout.String())
	if stdout == "" {
		return errors.Errorf("command %s completed successfully, but the response does not contain JSON string", command)
	}

	type awsCreds struct {
		AccessKeyID     string `json:"ACCESS_KEY_ID"`
		SecretAccessKey string `json:"SECRET_ACCESS_KEY"`
		SessionToken    string `json:"SESSION_TOKEN"`
	}

	type awsRole struct {
		RoleARN          string `json:"roleARN"`
		RoleSessionName  string `json:"roleSessionName"`
		WebIdentityToken string `json:"webIdentityToken"`
		Duration         int64  `json:"duration"`
	}

	var resp struct {
		AWSCredentials *awsCreds         `json:"awsCredentials"`
		AWSRole        *awsRole          `json:"awsRole"`
		Envs           map[string]string `json:"envs"`
	}

	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		return errors.Errorf("command %s returned a response with invalid JSON format", command)
	}

	if resp.Envs != nil {
		if opts.Env == nil {
			opts.Env = make(map[string]string)
		}

		for k, v := range resp.Envs {
			opts.Env[k] = v
		}
	}

	if resp.AWSCredentials != nil {
		if resp.AWSCredentials.AccessKeyID != "" && resp.AWSCredentials.SecretAccessKey != "" {
			if opts.Env == nil {
				opts.Env = make(map[string]string)
			}

			opts.Env["AWS_ACCESS_KEY_ID"] = resp.AWSCredentials.AccessKeyID
			opts.Env["AWS_SECRET_ACCESS_KEY"] = resp.AWSCredentials.SecretAccessKey
			opts.Env["AWS_SESSION_TOKEN"] = resp.AWSCredentials.SessionToken
			opts.Env["AWS_SECURITY_TOKEN"] = resp.AWSCredentials.SessionToken
		}

		return nil
	}

	if resp.AWSRole != nil && resp.AWSRole.RoleARN != "" {
		opts.IAMRoleOptions.RoleARN = resp.AWSRole.RoleARN
		if resp.AWSRole.RoleSessionName != "" {
			opts.IAMRoleOptions.AssumeRoleSessionName = resp.AWSRole.RoleSessionName
		}

		if resp.AWSRole.Duration > 0 {
			opts.IAMRoleOptions.AssumeRoleDuration = resp.AWSRole.Duration
		}

		if resp.AWSRole.WebIdentityToken != "" {
			opts.IAMRoleOptions.WebIdentityToken = resp.AWSRole.WebIdentityToken
		}
	}

	return nil
}

// AssumeIamRole assumes an IAM role and returns the credentials
func AssumeIamRole(
	ctx context.Context,
	iamRoleOpts options.IAMRoleOptions,
	externalID string,
	opts *options.TerragruntOptions,
) (*types.Credentials, error) {
	var region string
	if opts != nil {
		region = opts.Env["AWS_REGION"]
		if region == "" {
			region = opts.Env["AWS_DEFAULT_REGION"]
		}
	}

	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = os.Getenv("AWS_DEFAULT_REGION")
		}
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
			return nil, errors.Errorf("Error assuming role: %w", err)
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
			return nil, errors.Errorf("Error reading web identity token file: %w", err)
		}

		token = string(tb)
	}

	input := &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(iamRoleOpts.RoleARN),
		RoleSessionName:  aws.String(roleSessionName),
		WebIdentityToken: aws.String(token),
		DurationSeconds:  aws.Int32(int32(duration.Seconds())),
	}

	// Build STS config using region derived from opts and use anonymous credentials to avoid IMDS/default chain
	stsAnonCfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(region),
		config.WithAppID("terragrunt/"+version.GetVersion()),
	)
	if err != nil {
		return nil, errors.Errorf("Error loading AWS config: %w", err)
	}

	stsAnonCfg.Credentials = aws.AnonymousCredentials{}
	stsClient = sts.NewFromConfig(stsAnonCfg)

	result, err := stsClient.AssumeRoleWithWebIdentity(ctx, input)
	if err != nil {
		return nil, errors.Errorf("Error assuming role with web identity: %w", err)
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

	return aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		stsCfg := cfg
		stsCfg.Credentials = aws.AnonymousCredentials{}
		stsClient := sts.NewFromConfig(stsCfg)

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

// newWebIdentityProvider returns a credentials provider that calls AssumeRoleWithWebIdentity using the given region and role.
func newWebIdentityProvider(region string, iamRoleOptions options.IAMRoleOptions) aws.CredentialsProvider {
	return aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		stsCfg, err := config.LoadDefaultConfig(
			ctx,
			config.WithRegion(region),
			config.WithAppID("terragrunt/"+version.GetVersion()),
		)
		if err != nil {
			return aws.Credentials{}, err
		}

		stsCfg.Credentials = aws.AnonymousCredentials{}
		stsClient := sts.NewFromConfig(stsCfg)

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
			RoleSessionName:  aws.String(iamRoleOptions.AssumeRoleSessionName),
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
