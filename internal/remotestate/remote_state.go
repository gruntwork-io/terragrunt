// Package remotestate contains code for configuring remote state storage.
package remotestate

import (
	"context"
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
)

var backends = backend.Backends{
	s3.NewBackend(),
	gcs.NewBackend(),
}

// RemoteState is the configuration for Terraform remote state.
type RemoteState struct {
	*Config `mapstructure:",squash"`
	backend backend.Backend
}

// New creates a new `RemoteState` instance.
func New(config *Config) *RemoteState {
	remote := &RemoteState{
		Config:  config,
		backend: backend.NewCommonBackend(config.BackendName),
	}

	if backend := backends.Get(config.BackendName); backend != nil {
		remote.backend = backend
	}

	return remote
}

// String implements `fmt.Stringer` interface.
func (remote *RemoteState) String() string {
	return remote.Config.String()
}

func (remote *RemoteState) IsVersionControlEnabled(ctx context.Context, opts *options.TerragruntOptions) (bool, error) {
	opts.Logger.Debugf("Checking if version control is enabled for the %s backend", remote.BackendName)

	return remote.backend.IsVersionControlEnabled(ctx, remote.BackendConfig, opts)
}

// Delete deletes the remote state.
func (remote *RemoteState) Delete(ctx context.Context, opts *options.TerragruntOptions) error {
	opts.Logger.Debugf("Deleting remote state for the %s backend", remote.BackendName)

	return remote.backend.Delete(ctx, remote.BackendConfig, opts)
}

// DeleteBucket deletes the entire bucket.
func (remote *RemoteState) DeleteBucket(ctx context.Context, opts *options.TerragruntOptions) error {
	opts.Logger.Debugf("Deleting the entire bucket for the %s backend", remote.BackendName)

	return remote.backend.DeleteBucket(ctx, remote.BackendConfig, opts)
}

// Bootstrap performs any actions necessary to bootstrap remote state before it's used for storage. For example, if you're
// using S3 or GCS for remote state storage, this may create the bucket if it doesn't exist already.
func (remote *RemoteState) Bootstrap(ctx context.Context, opts *options.TerragruntOptions) error {
	opts.Logger.Debugf("Bootstrapping remote state for the %s backend", remote.BackendName)

	return remote.backend.Bootstrap(ctx, remote.BackendConfig, opts)
}

// Migrate determines where the remote state resources exist for source backend config and migrate them to dest backend config.
func (remote *RemoteState) Migrate(ctx context.Context, opts, dstOpts *options.TerragruntOptions, dstRemote *RemoteState) error {
	opts.Logger.Debugf("Migrate remote state for the %s backend", remote.BackendName)

	if remote.BackendName == dstRemote.BackendName {
		return remote.backend.Migrate(ctx, remote.BackendConfig, dstRemote.BackendConfig, opts)
	}

	return remote.pullPushState(ctx, opts, dstOpts)
}

// NeedsBootstrap returns true if remote state needs to be configured. This will be the case when:
//
// 1. Remote state auto-initialization has been disabled.
// 2. Remote state has not already been configured.
// 3. Remote state has been configured, but with a different configuration.
// 4. The remote state bootstrapper for this backend type, if there is one, says bootstrap is necessary.
func (remote *RemoteState) NeedsBootstrap(ctx context.Context, opts *options.TerragruntOptions) (bool, error) {
	if opts.DisableBucketUpdate {
		opts.Logger.Debug("Skipping remote state bootstrap")
		return false, nil
	}

	if remote.DisableInit {
		return false, nil
	}

	// The specific backend type will check if bootstrap is necessary.
	opts.Logger.Debugf("Checking if remote state bootstrap is necessary for the %s backend", remote.BackendName)

	return remote.backend.NeedsBootstrap(ctx, remote.BackendConfig, opts)
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

	var backendConfigArgs = make([]string, 0, len(config))

	for key, value := range config {
		arg := fmt.Sprintf("-backend-config=%s=%v", key, value)
		backendConfigArgs = append(backendConfigArgs, arg)
	}

	return backendConfigArgs
}

// GenerateOpenTofuCode generates the OpenTofu/Terraform code for configuring remote state backend.
func (remote *RemoteState) GenerateOpenTofuCode(opts *options.TerragruntOptions) error {
	backendConfig := remote.backend.GetTFInitArgs(remote.BackendConfig)

	return remote.Config.GenerateOpenTofuCode(opts, backendConfig)
}

func (remote *RemoteState) pullPushState(ctx context.Context, opts, dstOpts *options.TerragruntOptions) error {
	args := []string{tf.CommandNameState, tf.CommandNamePull}

	output, err := tf.RunCommandWithOutput(ctx, opts, args...)
	if err != nil {
		return err
	}

	file, err := os.CreateTemp("", "*.tfstate")
	if err != nil {
		errors.New(err)
	}

	if _, err := file.Write(output.Stdout.Bytes()); err != nil {
		errors.New(err)
	}

	args = []string{tf.CommandNameState, tf.CommandNamePush, file.Name()}

	return tf.RunCommand(ctx, dstOpts, args...)
}
