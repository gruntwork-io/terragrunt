package helper

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
)

// AssumeRoleAndUpdateEnvIfNecessary assumes an IAM role by the different methods:
// 1. attempting to run provider command if `--terragrunt-auth-provider-cmd` specified
// 2. attempting to make API calls to Amazon STS if `--terragrunt-iam-role` specified
// and finally setting the credentials we get back to `opts.Env`.
func AssumeRoleAndUpdateEnvIfNecessary(ctx context.Context, opts *options.TerragruntOptions) error {
	creds, err := getAWSCredentialsByRunningProviderCommand(ctx, opts)
	if err != nil {
		return err
	}

	if apiCreds, err := getAWSCredentialsByMakingAPICallsToAmazonSTS(opts); err != nil {
		return err
	} else if apiCreds != nil {
		if creds != nil {
			opts.Logger.Warnf("AWS credentials obtained using the %s command are overwritten by credentials obtained by assuming the %q role.", opts.AuthProviderCmd, opts.IAMRoleOptions.RoleARN)
		}
		creds = apiCreds
	}

	if creds == nil {
		return nil
	}

	opts.Env["AWS_ACCESS_KEY_ID"] = aws.StringValue(creds.AccessKeyId)
	opts.Env["AWS_SECRET_ACCESS_KEY"] = aws.StringValue(creds.SecretAccessKey)
	opts.Env["AWS_SESSION_TOKEN"] = aws.StringValue(creds.SessionToken)
	opts.Env["AWS_SECURITY_TOKEN"] = aws.StringValue(creds.SessionToken)

	return nil
}

func getAWSCredentialsByRunningProviderCommand(ctx context.Context, opts *options.TerragruntOptions) (*sts.Credentials, error) {
	command := opts.AuthProviderCmd
	if command == "" {
		return nil, nil
	}

	var args []string
	if parts := strings.Fields(command); len(parts) > 1 {
		command = parts[0]
		args = parts[1:]
	}

	output, err := shell.RunShellCommandWithOutput(ctx, opts, "", true, false, command, args...)
	if err != nil {
		return nil, err
	}

	if output.Stdout == "" {
		return nil, errors.Errorf("the command %s completed successfully, but the response does not contain json string", command)
	}

	type credentials struct {
		Envs struct {
			AccessKeyID     string `json:"AWS_ACCESS_KEY_ID"`
			SecretAccessKey string `json:"AWS_SECRET_ACCESS_KEY"`
			SessionToken    string `json:"AWS_SESSION_TOKEN"`
		} `json:"envs"`
	}
	creds := new(credentials)

	if err := json.Unmarshal([]byte(output.Stdout), &creds); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if creds.Envs.AccessKeyID == "" || creds.Envs.SecretAccessKey == "" || creds.Envs.SessionToken == "" {
		opts.Logger.Warnf("The command %s completed successfully, but the response contains empty credentials, nothing is being done.", command)
		return nil, nil
	}

	return &sts.Credentials{
		AccessKeyId:     &creds.Envs.AccessKeyID,
		SecretAccessKey: &creds.Envs.SecretAccessKey,
		SessionToken:    &creds.Envs.SessionToken,
	}, nil
}

func getAWSCredentialsByMakingAPICallsToAmazonSTS(opts *options.TerragruntOptions) (*sts.Credentials, error) {
	iamRoleOpts := opts.IAMRoleOptions
	if iamRoleOpts.RoleARN == "" {
		return nil, nil
	}

	opts.Logger.Debugf("Assuming IAM role %s with a session duration of %d seconds.", iamRoleOpts.RoleARN, iamRoleOpts.AssumeRoleDuration)
	creds, err := aws_helper.AssumeIamRole(iamRoleOpts)
	if err != nil {
		return nil, err
	}

	return creds, nil
}
