// Package externalcmd provides a provider that runs an external command that returns a json string with credentials.
package externalcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/mattn/go-shellwords"
)

// Provider runs external command that returns a json string with credentials.
type Provider struct {
	terragruntOptions *options.TerragruntOptions
}

// NewProvider returns a new Provider instance.
func NewProvider(l log.Logger, opts *options.TerragruntOptions) providers.Provider {
	return &Provider{
		terragruntOptions: opts,
	}
}

// Name implements providers.Name
func (provider *Provider) Name() string {
	return fmt.Sprintf("external %s command", provider.terragruntOptions.AuthProviderCmd)
}

// GetCredentials implements providers.GetCredentials
func (provider *Provider) GetCredentials(ctx context.Context, l log.Logger) (*providers.Credentials, error) {
	if provider.terragruntOptions.AuthProviderCmd == "" {
		return nil, nil
	}

	parser := shellwords.NewParser()

	parts, err := parser.Parse(provider.terragruntOptions.AuthProviderCmd)
	if err != nil {
		return nil, errors.Errorf("failed to parse auth provider command: %w", err)
	}

	command := parts[0]

	args := []string{}
	if len(parts) > 1 {
		args = parts[1:]
	}

	output, err := shell.RunCommandWithOutput(ctx, l, provider.terragruntOptions, "", true, false, command, args...)
	if err != nil {
		return nil, err
	}

	if output.Stdout.String() == "" {
		return nil, errors.Errorf(
			"command %s completed successfully, but the response does not contain JSON string",
			provider.terragruntOptions.AuthProviderCmd,
		)
	}

	resp := &Response{Envs: make(map[string]string)}

	if err := json.Unmarshal(output.Stdout.Bytes(), &resp); err != nil {
		return nil, errors.Errorf("command %s returned a response with invalid JSON format", command)
	}

	creds := &providers.Credentials{
		Name: providers.AWSCredentials,
		Envs: resp.Envs,
	}

	if resp.AWSCredentials != nil {
		if envs := resp.AWSCredentials.Envs(ctx, l, provider.terragruntOptions); envs != nil {
			l.Debugf("Obtaining AWS credentials from the %s.", provider.Name())
			maps.Copy(creds.Envs, envs)
		}

		return creds, nil
	}

	if resp.AWSRole != nil {
		if envs := resp.AWSRole.Envs(ctx, l, provider.terragruntOptions); envs != nil {
			l.Debugf("Assuming AWS role %s using the %s.", resp.AWSRole.RoleARN, provider.Name())
			maps.Copy(creds.Envs, envs)
		}

		return creds, nil
	}

	return creds, nil
}

type Response struct {
	AWSCredentials *AWSCredentials   `json:"awsCredentials"`
	AWSRole        *AWSRole          `json:"awsRole"`
	Envs           map[string]string `json:"envs"`
}

type AWSCredentials struct {
	AccessKeyID     string `json:"ACCESS_KEY_ID"`
	SecretAccessKey string `json:"SECRET_ACCESS_KEY"`
	SessionToken    string `json:"SESSION_TOKEN"`
}

type AWSRole struct {
	RoleARN          string `json:"roleARN"`
	RoleSessionName  string `json:"roleSessionName"`
	WebIdentityToken string `json:"webIdentityToken"`
	Duration         int64  `json:"duration"`
}

func (role *AWSRole) Envs(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) map[string]string {
	if role.RoleARN == "" {
		l.Warnf("The command %s completed successfully, but AWS role assumption contains empty required value: roleARN, nothing is being done.", opts.AuthProviderCmd)
		return nil
	}

	sessionName := role.RoleSessionName
	if sessionName == "" {
		sessionName = options.GetDefaultIAMAssumeRoleSessionName()
	}

	duration := role.Duration
	if duration == 0 {
		duration = options.DefaultIAMAssumeRoleDuration
	}

	// Construct minimal TerragruntOptions for role assumption.
	providerOpts := options.TerragruntOptions{
		IAMRoleOptions: options.IAMRoleOptions{
			RoleARN:               role.RoleARN,
			AssumeRoleDuration:    duration,
			AssumeRoleSessionName: sessionName,
		},
	}

	if role.WebIdentityToken != "" {
		providerOpts.IAMRoleOptions.WebIdentityToken = role.WebIdentityToken
	}

	provider := amazonsts.NewProvider(l, &providerOpts)

	creds, err := provider.GetCredentials(ctx, l)
	if err != nil {
		l.Warnf("Failed to assume role %s: %v", role.RoleARN, err)
		return nil
	}

	if creds == nil {
		l.Warnf("The command %s completed successfully, but failed to assume role %s, nothing is being done.", opts.AuthProviderCmd, role.RoleARN)
		return nil
	}

	envs := map[string]string{
		"AWS_ACCESS_KEY_ID":     creds.Envs["AWS_ACCESS_KEY_ID"],
		"AWS_SECRET_ACCESS_KEY": creds.Envs["AWS_SECRET_ACCESS_KEY"],
		"AWS_SESSION_TOKEN":     creds.Envs["AWS_SESSION_TOKEN"],
		"AWS_SECURITY_TOKEN":    creds.Envs["AWS_SESSION_TOKEN"],
	}

	return envs
}

func (creds *AWSCredentials) Envs(_ context.Context, l log.Logger, opts *options.TerragruntOptions) map[string]string {
	var emptyFields []string

	if creds.AccessKeyID == "" {
		emptyFields = append(emptyFields, "ACCESS_KEY_ID")
	}

	if creds.SecretAccessKey == "" {
		emptyFields = append(emptyFields, "SECRET_ACCESS_KEY")
	}

	if len(emptyFields) > 0 {
		l.Warnf("The command %s completed successfully, but AWS credentials contains empty required values: %s, nothing is being done.", opts.AuthProviderCmd, strings.Join(emptyFields, ", "))
		return nil
	}

	envs := map[string]string{
		"AWS_ACCESS_KEY_ID":     creds.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": creds.SecretAccessKey,
		"AWS_SESSION_TOKEN":     creds.SessionToken,
		"AWS_SECURITY_TOKEN":    creds.SessionToken,
	}

	return envs
}
