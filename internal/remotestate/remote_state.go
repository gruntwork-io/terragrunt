// Package remotestate contains code for configuring remote state storage.
package remotestate

import (
	"context"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/options"
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

// DeleteBucket deletes the remote state.
func (remote *RemoteState) DeleteBucket(ctx context.Context, opts *options.TerragruntOptions) error {
	opts.Logger.Debugf("Deleting remote state for the %s backend", remote.BackendName)

	return remote.backend.DeleteBucket(ctx, remote.BackendConfig, opts)
}

// Init performs any actions necessary to initialize the remote state before it's used for storage. For example, if you're
// using S3 or GCS for remote state storage, this may create the bucket if it doesn't exist already.
func (remote *RemoteState) Init(ctx context.Context, opts *options.TerragruntOptions) error {
	opts.Logger.Debugf("Initializing remote state for the %s backend", remote.BackendName)

	return remote.backend.Init(ctx, remote.BackendConfig, opts)
}

// NeedsInit returns true if remote state needs to be configured. This will be the case when:
//
// 1. Remote state auto-initialization has been disabled.
// 2. Remote state has not already been configured.
// 3. Remote state has been configured, but with a different configuration.
// 4. The remote state initializer for this backend type, if there is one, says initialization is necessary.
func (remote *RemoteState) NeedsInit(ctx context.Context, opts *options.TerragruntOptions) (bool, error) {
	if opts.DisableBucketUpdate {
		opts.Logger.Debug("Skipping remote state initialization")
		return false, nil
	}

	if remote.DisableInit {
		return false, nil
	}

	tfState, err := ParseTerraformStateFileFromLocation(remote.BackendName, remote.BackendConfig, opts.WorkingDir, opts.DataDir())
	if err != nil {
		return false, err
	}

	// Remote state not configured.
	if tfState == nil {
		return true, nil
	}

	if len(tfState.Backend.Config) == 0 && len(remote.Config.BackendConfig) != 0 {
		return true, nil
	}

	if tfState.Backend.Type != remote.backend.Name() {
		opts.Logger.Debugf("Backend type has changed from %s to %s", remote.backend.Name(), tfState.Backend.Type)

		return true, nil
	}

	if !tfState.IsRemote() {
		return false, nil
	}

	// Remote state initializer says initialization is necessary.
	return remote.backend.NeedsInit(ctx, remote.BackendConfig, tfState.Backend.Config, opts)
}

// ToTerraformInitArgs converts the RemoteState config into the format used by the terraform init command.
func (remote *RemoteState) ToTerraformInitArgs() []string {
	if remote.DisableInit {
		return []string{"-backend=false"}
	}

	if remote.Generate != nil {
		// When in generate mode, we don't need to use `-backend-config` to initialize the remote state backend.
		return []string{}
	}

	config := remote.backend.GetTerraformInitArgs(remote.BackendConfig)

	var backendConfigArgs = make([]string, 0, len(config))

	for key, value := range config {
		arg := fmt.Sprintf("-backend-config=%s=%v", key, value)
		backendConfigArgs = append(backendConfigArgs, arg)
	}

	return backendConfigArgs
}

// GenerateTerraformCode generates the terraform code for configuring remote state backend.
func (remote *RemoteState) GenerateTerraformCode(opts *options.TerragruntOptions) error {
	backendConfig := remote.backend.GetTerraformInitArgs(remote.BackendConfig)

	return remote.Config.GenerateTerraformCode(opts, backendConfig)
}
