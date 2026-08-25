package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// s3DirectStateReadSupported preserves the native backend's validation for the
// configurable workspace prefix. An explicit empty prefix is valid; leading or
// trailing slashes are not.
func s3DirectStateReadSupported(pctx *ParsingContext, config backend.Config) bool {
	// SSE-C state needs the customer key on every GetObject. The direct reader does
	// not send those headers, so AWS would answer 400 for state the native backend
	// reads fine.
	if backendConfigValueSet(config, "sse_customer_key") || pctx.Venv.Env["AWS_SSE_CUSTOMER_KEY"] != "" {
		return false
	}

	value, configured := config["workspace_key_prefix"]
	if !configured {
		return true
	}

	prefix, ok := value.(string)
	if !ok {
		return false
	}

	return prefix == "" || (!strings.HasPrefix(prefix, "/") && !strings.HasSuffix(prefix, "/"))
}

// getTerragruntOutputJSONFromRemoteStateS3 pulls the output directly from an S3 bucket without calling Terraform
func getTerragruntOutputJSONFromRemoteStateS3(
	ctx context.Context,
	l log.Logger,
	pctx *ParsingContext,
	remoteState *remotestate.RemoteState,
	workspace string,
) ([]byte, error) {
	bucket := fmt.Sprintf("%s", remoteState.BackendConfig["bucket"])
	key := s3StateObjectKey(remoteState.BackendConfig, workspace)
	location := fmt.Sprintf("s3://%s/%s", bucket, key)

	open := func(ctx context.Context, l log.Logger) (io.ReadCloser, error) {
		s3ConfigExtended, err := s3backend.Config(remoteState.BackendConfig).ParseExtendedS3Config()
		if err != nil {
			return nil, fmt.Errorf("parsing s3 backend config for %s: %w", location, err)
		}

		s3Client, err := awshelper.NewAWSConfigBuilder().
			WithSessionConfig(s3ConfigExtended.GetAwsSessionConfig()).
			WithIAMRoleOptions(pctx.IAMRoleOptions).
			BuildS3Client(ctx, l, pctx.Venv)
		if err != nil {
			return nil, fmt.Errorf("building s3 client for %s: %w", location, err)
		}

		result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return nil, fmt.Errorf("fetching dependency state from %s: %w", location, err)
		}

		return result.Body, nil
	}

	return readDependencyStateOutputs(ctx, l, "dependency_output_state_s3", map[string]any{
		"bucket": bucket,
		"key":    key,
	}, location, open)
}

// s3StateObjectKey mirrors the S3 backend's workspace object layout.
func s3StateObjectKey(config backend.Config, workspace string) string {
	key := fmt.Sprintf("%s", config["key"])
	if workspace == defaultStateWorkspace {
		return key
	}

	prefix := defaultS3WorkspacePath
	if configured, ok := config["workspace_key_prefix"].(string); ok {
		prefix = configured
	}

	return path.Join(prefix, workspace, key)
}

// isAwsS3StateMissing reports whether err means the dependency's S3 state object or bucket doesn't
// exist yet, the signal to fall back to mock outputs. It matches on the error code because GetObject
// returns NoSuchBucket as a generic API error, not a *s3types.NoSuchBucket that errors.As could
// match; AWS SDK v2 exposes no constants for the codes.
func isAwsS3StateMissing(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	switch apiErr.ErrorCode() {
	case "NoSuchKey", "NoSuchBucket", "NotFound":
		return true
	default:
		return false
	}
}
