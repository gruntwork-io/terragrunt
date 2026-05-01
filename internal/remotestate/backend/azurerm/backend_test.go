package azurerm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func validBackendConfig() backend.Config {
	return backend.Config{
		keyStorageAccount: testStorageAccount,
		keyContainer:      testContainer,
		keyKey:            testKey,
		keyResourceGroup:  testRG,
	}
}

func disabledExperimentOpts() *backend.Options {
	return &backend.Options{Experiments: experiment.NewExperiments()}
}

func enabledExperimentOpts(t *testing.T) *backend.Options {
	t.Helper()

	exps := experiment.NewExperiments()
	if err := exps.EnableExperiment(experiment.AzureBackend); err != nil {
		t.Fatalf("EnableExperiment: %v", err)
	}

	return &backend.Options{Experiments: exps}
}

func TestBackendName(t *testing.T) {
	t.Parallel()

	if got := azurerm.NewBackend().Name(); got != azurerm.BackendName {
		t.Fatalf("Name() = %q, want %q", got, azurerm.BackendName)
	}
}

// TestBackend_LifecycleGatedByExperiment asserts that every lifecycle entry
// point returns ExperimentNotEnabledError when the azure-backend experiment
// is off, regardless of whether the supplied config is otherwise valid.
// This is the contract the PR-1 stub established and PR-3 must preserve so
// that an existing azurerm config in a terragrunt.hcl keeps round-tripping
// through to terraform without any Azure SDK calls.
func TestBackend_LifecycleGatedByExperiment(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurerm.NewBackend()
	cfg := validBackendConfig()
	opts := disabledExperimentOpts()

	t.Run("NeedsBootstrap", func(t *testing.T) {
		t.Parallel()

		needs, err := b.NeedsBootstrap(ctx, l, cfg, opts)
		assertExperimentNotEnabled(t, err)

		if needs {
			t.Errorf("NeedsBootstrap = true, want false on disabled experiment")
		}
	})

	t.Run("Bootstrap", func(t *testing.T) {
		t.Parallel()
		assertExperimentNotEnabled(t, b.Bootstrap(ctx, l, cfg, opts))
	})

	t.Run("IsVersionControlEnabled", func(t *testing.T) {
		t.Parallel()

		on, err := b.IsVersionControlEnabled(ctx, l, cfg, opts)
		assertExperimentNotEnabled(t, err)

		if on {
			t.Errorf("IsVersionControlEnabled = true, want false on disabled experiment")
		}
	})

	t.Run("Migrate", func(t *testing.T) {
		t.Parallel()
		assertExperimentNotEnabled(t, b.Migrate(ctx, l, cfg, cfg, opts))
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		assertExperimentNotEnabled(t, b.Delete(ctx, l, cfg, opts))
	})

	t.Run("DeleteBucket", func(t *testing.T) {
		t.Parallel()
		assertExperimentNotEnabled(t, b.DeleteBucket(ctx, l, cfg, opts))
	})
}

// TestBackend_LifecycleEnabledExperimentSurfaceConfigErrors asserts that
// once the experiment is enabled, the lifecycle methods do reach config
// validation: passing an empty config should produce a missing-required
// error rather than ExperimentNotEnabledError.
func TestBackend_LifecycleEnabledExperimentSurfaceConfigErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := logger.CreateLogger()
	b := azurerm.NewBackend()
	opts := enabledExperimentOpts(t)
	emptyCfg := backend.Config{}

	err := b.Bootstrap(ctx, l, emptyCfg, opts)
	if err == nil {
		t.Fatal("Bootstrap with empty config: want error, got nil")
	}

	var notEnabled azurerm.ExperimentNotEnabledError

	if errors.As(err, &notEnabled) {
		t.Errorf("Bootstrap returned ExperimentNotEnabledError when experiment was enabled: %v", err)
	}
}

// TestBackend_GetTFInitArgsAvailableWithoutExperiment confirms the
// terraform-passthrough behaviour established in PR 1: GetTFInitArgs works
// regardless of experiment state, since it must run during terragrunt init
// even when the user has not opted into the azure-backend experiment.
func TestBackend_GetTFInitArgsAvailableWithoutExperiment(t *testing.T) {
	t.Parallel()

	args := azurerm.NewBackend().GetTFInitArgs(validBackendConfig())

	if got := args[keyStorageAccount]; got != testStorageAccount {
		t.Errorf("storage_account_name = %v, want %s", got, testStorageAccount)
	}

	if _, ok := args["location"]; ok {
		t.Errorf("expected terragrunt-only location key to be filtered out")
	}
}

func assertExperimentNotEnabled(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected ExperimentNotEnabledError, got nil")
	}

	var target azurerm.ExperimentNotEnabledError
	if !errors.As(err, &target) {
		t.Fatalf("expected ExperimentNotEnabledError, got %T: %v", err, err)
	}
}
