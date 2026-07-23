// Package experiment provides utilities used by Terragrunt to support an "experiment" mode.
// By default, experiment mode is disabled, but when enabled, experimental features can be enabled.
// These features are not yet stable and may change in the future.
//
// Note that any behavior outlined here should be documented in /docs/_docs/04_reference/experiments.md
//
// That is how users will know what to expect when they enable experiment mode, and how to customize it.
package experiment

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// Symlinks is the experiment that allows symlinks to be used in Terragrunt configurations.
	Symlinks = "symlinks"
	// CLIRedesign is an experiment that allows users to use new commands related to the CLI redesign.
	CLIRedesign = "cli-redesign"
	// Stacks is the experiment that allows stacks to be used in Terragrunt.
	Stacks = "stacks"
	// CAS names the now-stable content-addressable storage behavior. CAS speeds up
	// catalog cloning, OpenTofu/Terraform source cloning, and stack generation by
	// avoiding redundant downloads of Git repositories. It is enabled by default and
	// can be disabled with the --no-cas flag.
	CAS = "cas"
	// Report is the experiment that enables the new run report.
	Report = "report"
	// RunnerPool is the experiment that allows using a pool of runners for parallel execution.
	RunnerPool = "runner-pool"
	// AutoProviderCacheDir is the experiment that automatically enables central
	// provider caching by setting TF_PLUGIN_CACHE_DIR.
	//
	// Only works with OpenTofu version >= 1.10.
	AutoProviderCacheDir = "auto-provider-cache-dir"
	// FilterFlag is the experiment that enables usage of the filter flag for filtering components
	FilterFlag = "filter-flag"
	// IacEngine is the experiment that enables usage of Terragrunt IaC engines for running IaC operations.
	IacEngine = "iac-engine"
	// DependencyFetchOutputFromState is the experiment that enables fetching dependency outputs
	// directly from state files instead of using terraform/tofu output commands.
	DependencyFetchOutputFromState = "dependency-fetch-output-from-state"
	// SlowTaskReporting enables progress spinners and completion logs for long-running operations.
	SlowTaskReporting = "slow-task-reporting"
	// DAGQueueDisplay is the experiment that shows the run queue as a DAG tree
	// with dependency hierarchy instead of a flat list.
	DAGQueueDisplay = "dag-queue-display"
	// StackDependencies is the experiment that enables the autoinclude block
	// in terragrunt.stack.hcl files, allowing units and stacks to define
	// dependency relationships and arbitrary configuration overrides during
	// stack generation. See RFC #5663.
	StackDependencies = "stack-dependencies"
	// CatalogRedesign names the now-default catalog experience: whole-repository
	// discovery, tabbed browsing, and an interactive scaffolding form. It is no
	// longer gated and the flag is retained only for backwards compatibility.
	CatalogRedesign = "catalog-redesign"
	// MarkManyAsRead names the now-stable behaviors that mark many files as
	// read in one step: automatic marking of files inside a local terraform
	// module source (so reading-based filter expressions detect changes to
	// the module) and the mark_glob_as_read HCL function. Both are enabled
	// by default.
	MarkManyAsRead = "mark-many-as-read"
	// AzureBackend reserves the experiment flag for native Azure Storage (azurerm)
	// remote state support. The backend is stubbed out for now; full Azure helper
	// and state-management behavior (bootstrap, delete, migrate, dependency output
	// fetching) will land in follow-up PRs.
	AzureBackend = "azure-backend"
	// DeepMerge enables the deep_merge HCL function.
	DeepMerge = "deep-merge"
	// OptOutAuth names the now-stable flags that opt out of running
	// --auth-provider-cmd in specific phases. The
	// --no-discovery-auth-provider-cmd flag is enabled by default.
	OptOutAuth = "opt-out-auth"
	// HookContextEnv exposes additional TG_CTX_* environment variables to hook
	// scripts: TG_CTX_HOOK_TYPE, TG_CTX_SOURCE, and TG_CTX_TERRAGRUNT_DIR.
	HookContextEnv = "hook-context-env"
	// OptionalHooks gates flags that make Terragrunt hooks optional during runs.
	OptionalHooks = "optional-hooks"
	// OCI gates downloading modules from OCI Distribution registries via oci:// sources.
	OCI = "oci"
	// VersionAttribute gates resolving a tfr:// registry module from a version
	// constraint expressed through the version attribute on the terraform block.
	VersionAttribute = "version-attribute"
	// OtelLogs enables the OpenTelemetry logs signal, exporting Terragrunt's log
	// records through the configured logs exporter and correlating them with
	// traces via the active span.
	OtelLogs = "otel-logs"
	// PatchSourceOutOfCache gates the --tf-update-source-out-of-cache flag, which
	// rewrites relative paths in a source-less unit's OpenTofu/Terraform files so
	// they still resolve after the files are copied into the .terragrunt-cache
	// directory.
	PatchSourceOutOfCache = "patch-source-out-of-cache"
)

const (
	// StatusOngoing is the status of an experiment that is ongoing.
	StatusOngoing byte = iota
	// StatusCompleted is the status of an experiment that is completed.
	StatusCompleted
)

type Experiments []*Experiment

// NewExperiments returns a new Experiments map with all experiments disabled.
//
// Bottom values for each experiment are the defaults, so only the names of experiments need to be set.
func NewExperiments() Experiments {
	return Experiments{
		{
			Name: Symlinks,
		},
		{
			Name:   CLIRedesign,
			Status: StatusCompleted,
		},
		{
			Name:   Stacks,
			Status: StatusCompleted,
		},
		{
			Name:   CAS,
			Status: StatusCompleted,
		},
		{
			Name:   Report,
			Status: StatusCompleted,
		},
		{
			Name:   RunnerPool,
			Status: StatusCompleted,
		},
		{
			Name:   AutoProviderCacheDir,
			Status: StatusCompleted,
		},
		{
			Name:   FilterFlag,
			Status: StatusCompleted,
		},
		{
			Name: IacEngine,
		},
		{
			Name: DependencyFetchOutputFromState,
		},
		{
			Name: SlowTaskReporting,
		},
		{
			Name:   DAGQueueDisplay,
			Status: StatusCompleted,
		},
		{
			Name:   StackDependencies,
			Status: StatusCompleted,
		},
		{
			Name:   CatalogRedesign,
			Status: StatusCompleted,
		},
		{
			Name:   MarkManyAsRead,
			Status: StatusCompleted,
		},
		{
			Name: AzureBackend,
		},
		{
			Name: DeepMerge,
		},
		{
			Name:   OptOutAuth,
			Status: StatusCompleted,
		},
		{
			Name: HookContextEnv,
		},
		{
			Name: OptionalHooks,
		},
		{
			Name: OCI,
		},
		{
			Name: VersionAttribute,
		},
		{
			Name: OtelLogs,
		},
		{
			Name: PatchSourceOutOfCache,
		},
	}
}

// Names returns all experiment names.
func (exps Experiments) Names() []string {
	names := make([]string, 0, len(exps))

	for _, exp := range exps {
		names = append(names, exp.Name)
	}

	slices.Sort(names)

	return names
}

// FilterByStatus returns experiments filtered by the given `status`.
func (exps Experiments) FilterByStatus(status byte) Experiments {
	var found Experiments

	for _, experiment := range exps {
		if experiment.Status == status {
			found = append(found, experiment)
		}
	}

	return found
}

// Find searches and returns the experiment by the given `name`.
func (exps Experiments) Find(name string) *Experiment {
	for _, experiment := range exps {
		if experiment.Name == name {
			return experiment
		}
	}

	return nil
}

// ExperimentMode enables the experiment mode.
func (exps Experiments) ExperimentMode() {
	for _, e := range exps {
		if e.Status == StatusOngoing {
			e.Enabled = true
		}
	}
}

// EnableExperiment validates that the specified experiment name is valid and enables this experiment.
func (exps Experiments) EnableExperiment(name string) error {
	for _, e := range exps {
		if e.Name == name {
			e.Enabled = true
			return nil
		}
	}

	return NewInvalidExperimentNameError(exps.FilterByStatus(StatusOngoing).Names())
}

// NotifyCompletedExperiments logs the experiment names that are Enabled and have completed Status.
func (exps Experiments) NotifyCompletedExperiments(logger log.Logger) {
	var completed Experiments

	for _, experiment := range exps.FilterByStatus(StatusCompleted) {
		if experiment.Enabled {
			completed = append(completed, experiment)
		}
	}

	if len(completed) == 0 {
		return
	}

	logger.Warnf("%s", NewCompletedExperimentsWarning(completed.Names()).String())
}

// Evaluate returns true if the experiment is found and evaluates to true, otherwise returns false.
//
// Completed experiments evaluate to true, per [Experiment.Evaluate].
func (exps Experiments) Evaluate(name string) bool {
	if experiment := exps.Find(name); experiment != nil {
		return experiment.Evaluate()
	}

	return false
}

// Experiment represents an experiment that can be enabled.
// When the experiment is enabled, Terragrunt will behave in a way that uses some experimental functionality.
type Experiment struct {
	// Name is the name of the experiment.
	Name string
	// Enabled determines if the experiment is enabled.
	Enabled bool
	// Status is the status of the experiment.
	Status byte
}

func (exps Experiment) String() string {
	return exps.Name
}

// Evaluate returns true the experiment is enabled.
//
// If the experiment is completed, consider it permanently enabled.
func (exps Experiment) Evaluate() bool {
	return exps.Enabled || exps.Status == StatusCompleted
}
