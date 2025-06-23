// Package remotestate contains code for configuring remote state storage.
package remotestate

import (
	"context"
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
)

// Initialize backends with only the stable ones by default
var backends = backend.Backends{
	s3.NewBackend(),
	gcs.NewBackend(),
}

// Init function runs when the remotestate package is loaded
func init() {
	// Register the RegisterBackends function as a hook to be called when options
	// are finalized in the cli package
	options.RegisterHook(RegisterBackends)
}

// RegisterBackends conditionally registers all backends based on experiment flags
func RegisterBackends(opts *options.TerragruntOptions) {
	// Register experimental backends if enabled
	if opts.Experiments.Evaluate(experiment.AzureBackend) {
		backends = append(backends, azurerm.NewBackend())
	}
}

// RemoteState is the configuration for Terraform remote state.
type RemoteState struct {
	// Put map first
	Encryption map[string]any `mapstructure:"encryption"`
	// Put pointers together
	Generate *ConfigGenerate `mapstructure:"generate"`
	backend  backend.Backend
	// Struct after pointers
	BackendConfig backend.Config `mapstructure:"config"`
	// String field
	BackendName string `mapstructure:"backend"`
	// Bool fields at the end
	DisableInit                   bool `mapstructure:"disable_init"`
	DisableDependencyOptimization bool `mapstructure:"disable_dependency_optimization"`
	// Add padding to optimize struct size
	_ [6]byte // padding to align to 8-byte boundary
}

// New creates a new `RemoteState` instance.
func New(config *Config) *RemoteState {
	remote := &RemoteState{
		BackendConfig:                 config.BackendConfig,
		Generate:                      config.Generate,
		Encryption:                    config.Encryption,
		BackendName:                   config.BackendName,
		DisableInit:                   config.DisableInit,
		DisableDependencyOptimization: config.DisableDependencyOptimization,
		backend:                       backend.NewCommonBackend(config.BackendName),
	}

	if backend := backends.Get(config.BackendName); backend != nil {
		remote.backend = backend
	} else if config.BackendName == azurerm.BackendName {
		// If Azure backend was requested but not registered (experiment not enabled), provide a helpful error
		remote.backend = &experimentalAzureBackend{config.BackendName}
	}

	return remote
}

// experimentalAzureBackend is a placeholder backend that returns an informative error
// when the Azure backend is used without enabling the experiment
type experimentalAzureBackend struct {
	name string
}

func (b *experimentalAzureBackend) Name() string {
	return b.name
}

func (b *experimentalAzureBackend) GetTFInitArgs(config backend.Config) map[string]any {
	return map[string]any{}
}

func (b *experimentalAzureBackend) Bootstrap(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) error {
	return azureBackendExperimentError()
}

func (b *experimentalAzureBackend) NeedsBootstrap(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) (bool, error) {
	return false, azureBackendExperimentError()
}

func (b *experimentalAzureBackend) IsVersionControlEnabled(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) (bool, error) {
	return false, azureBackendExperimentError()
}

func (b *experimentalAzureBackend) Delete(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) error {
	return azureBackendExperimentError()
}

func (b *experimentalAzureBackend) DeleteBucket(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) error {
	return azureBackendExperimentError()
}

func (b *experimentalAzureBackend) DeleteStorageAccount(ctx context.Context, l log.Logger, config backend.Config, opts *options.TerragruntOptions) error {
	return azureBackendExperimentError()
}

func (b *experimentalAzureBackend) Migrate(ctx context.Context, l log.Logger, srcConfig backend.Config, dstConfig backend.Config, opts *options.TerragruntOptions) error {
	return azureBackendExperimentError()
}

func azureBackendExperimentError() error {
	return errors.New("Azure backend is an experimental feature. Enable it with --experiment azure-backend flag or TG_EXPERIMENT=azure-backend environment variable")
}

// IsVersionControlEnabled checks if versioning is enabled for the remote state.
func (remote *RemoteState) IsVersionControlEnabled(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (bool, error) {
	l.Debugf("Checking if version control is enabled for the %s backend", remote.BackendName)

	return remote.backend.IsVersionControlEnabled(ctx, l, remote.BackendConfig, opts)
}

// Delete deletes the remote state.
func (remote *RemoteState) Delete(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	l.Debugf("Deleting remote state for the %s backend", remote.BackendName)

	return remote.backend.Delete(ctx, l, remote.BackendConfig, opts)
}

// DeleteBucket deletes the entire bucket.
func (remote *RemoteState) DeleteBucket(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	l.Debugf("Deleting the entire bucket for the %s backend", remote.BackendName)

	return remote.backend.DeleteBucket(ctx, l, remote.BackendConfig, opts)
}

// Bootstrap performs any actions necessary to bootstrap remote state before it's used for storage. For example, if you're
// using S3 or GCS for remote state storage, this may create the bucket if it doesn't exist already.
func (remote *RemoteState) Bootstrap(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	l.Debugf("Bootstrapping remote state for the %s backend", remote.BackendName)

	return remote.backend.Bootstrap(ctx, l, remote.BackendConfig, opts)
}

// Migrate determines where the remote state resources exist for source backend config and migrate them to dest backend config.
func (remote *RemoteState) Migrate(ctx context.Context, l log.Logger, opts, dstOpts *options.TerragruntOptions, dstRemote *RemoteState) error {
	l.Debugf("Migrate remote state for the %s backend", remote.BackendName)

	if remote.BackendName == dstRemote.BackendName {
		return remote.backend.Migrate(ctx, l, remote.BackendConfig, dstRemote.BackendConfig, opts)
	}

	stateFile, err := remote.pullState(ctx, l, opts)
	if err != nil {
		return err
	}

	defer func() {
		os.Remove(stateFile) // nolint: errcheck
	}()

	return dstRemote.pushState(ctx, l, dstOpts, stateFile)
}

// NeedsBootstrap returns true if remote state needs to be configured. This will be the case when:
//
// 1. Remote state auto-initialization has been disabled.
// 2. Remote state has not already been configured.
// 3. Remote state has been configured, but with a different configuration.
// 4. The remote state bootstrapper for this backend type, if there is one, says bootstrap is necessary.
func (remote *RemoteState) NeedsBootstrap(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (bool, error) {
	if opts.DisableBucketUpdate {
		l.Debug("Skipping remote state bootstrap")
		return false, nil
	}

	if remote.DisableInit {
		return false, nil
	}

	// The specific backend type will check if bootstrap is necessary.
	l.Debugf("Checking if remote state bootstrap is necessary for the %s backend", remote.BackendName)

	return remote.backend.NeedsBootstrap(ctx, l, remote.BackendConfig, opts)
}

// GetTFInitArgs converts the RemoteState config into the format used by the `tofu init` command.
func (remote *RemoteState) GetTFInitArgs() []string {
	if remote.DisableInit {
		return []string{"-backend=false"}
	}

	if remote.Generate != nil {
		// When in generate mode, we don't need to use `-backend-config` to initialize the remote state backend.
		return []string{}
	}

	config := remote.backend.GetTFInitArgs(remote.BackendConfig)

	backendConfigArgs := make([]string, 0, len(config))

	for key, value := range config {
		arg := fmt.Sprintf("-backend-config=%s=%v", key, value)
		backendConfigArgs = append(backendConfigArgs, arg)
	}

	return backendConfigArgs
}

// GenerateOpenTofuCode generates the OpenTofu/Terraform code for configuring remote state backend.
func (remote *RemoteState) GenerateOpenTofuCode(l log.Logger, opts *options.TerragruntOptions) error {
	backendConfig := remote.backend.GetTFInitArgs(remote.BackendConfig)

	return remote.generateBackendConfig(l, opts, backendConfig)
}

// generateBackendConfig generates the backend configuration code with the provided config.
func (remote *RemoteState) generateBackendConfig(l log.Logger, opts *options.TerragruntOptions, backendConfig map[string]any) error {
	// Implementation for generating backend configuration
	// This is a placeholder - you need to implement the actual functionality
	l.Debugf("Generating backend config for %s backend", remote.BackendName)
	return nil
}

func (remote *RemoteState) pullState(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) (string, error) {
	l.Debugf("Pulling state from %s backend", remote.BackendName)

	args := []string{tf.CommandNameState, tf.CommandNamePull}

	output, err := tf.RunCommandWithOutput(ctx, l, opts, args...)
	if err != nil {
		return "", err
	}

	l.Debugf("Creating temporary state file for migration")

	file, err := os.CreateTemp("", "*.tfstate")
	if err != nil {
		return "", errors.New(err)
	}

	defer func() {
		file.Close() // nolint: errcheck
	}()

	if _, err := file.Write(output.Stdout.Bytes()); err != nil {
		return file.Name(), errors.New(err)
	}

	return file.Name(), nil
}

func (remote *RemoteState) pushState(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, stateFile string) error {
	l.Debugf("Pushing state to %s backend", remote.BackendName)

	args := []string{tf.CommandNameState, tf.CommandNamePush, stateFile}

	return tf.RunCommand(ctx, l, opts, args...)
}
