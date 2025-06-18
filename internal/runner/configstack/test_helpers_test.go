package configstack_test

import (
	"context"
	"sort"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TerraformModuleByPath common.Units

func (byPath TerraformModuleByPath) Len() int           { return len(byPath) }
func (byPath TerraformModuleByPath) Swap(i, j int)      { byPath[i], byPath[j] = byPath[j], byPath[i] }
func (byPath TerraformModuleByPath) Less(i, j int) bool { return byPath[i].Path < byPath[j].Path }

type DependencyControllerByPath []*configstack.DependencyController

func (byPath DependencyControllerByPath) Len() int      { return len(byPath) }
func (byPath DependencyControllerByPath) Swap(i, j int) { byPath[i], byPath[j] = byPath[j], byPath[i] }
func (byPath DependencyControllerByPath) Less(i, j int) bool {
	return byPath[i].Runner.Module.Path < byPath[j].Runner.Module.Path
}

// We can't use assert.Equals on TerraformModule or any data structure that contains it because it contains some
// fields (e.g. TerragruntOptions) that cannot be compared directly
func assertModuleListsEqual(t *testing.T, expectedModules common.Units, actualModules common.Units, messageAndArgs ...any) {
	t.Helper()

	if !assert.Len(t, actualModules, len(expectedModules), messageAndArgs...) {
		t.Logf("%s != %s", expectedModules, actualModules)
		return
	}

	sort.Sort(TerraformModuleByPath(expectedModules))
	sort.Sort(TerraformModuleByPath(actualModules))

	for i := range expectedModules {
		expected := expectedModules[i]
		actual := actualModules[i]
		assertModulesEqual(t, expected, actual, messageAndArgs...)
	}
}

// We can't use assert.Equals on TerraformModule because it contains some fields (e.g. TerragruntOptions) that cannot
// be compared directly
func assertModulesEqual(t *testing.T, expected *common.Unit, actual *common.Unit, messageAndArgs ...any) {
	t.Helper()

	if assert.NotNil(t, actual, messageAndArgs...) {
		// When comparing the TerragruntConfig objects, we need to normalize the dependency list to explicitly set the
		// expected to empty list when nil, as the parsing routine will set it to empty list instead of nil.
		if expected.Config.TerragruntDependencies == nil {
			expected.Config.TerragruntDependencies = config.Dependencies{}
		}
		if actual.Config.TerragruntDependencies == nil {
			actual.Config.TerragruntDependencies = config.Dependencies{}
		}
		assert.Equal(t, expected.Config, actual.Config, messageAndArgs...)

		assert.Equal(t, expected.Path, actual.Path, messageAndArgs...)
		assert.Equal(t, expected.AssumeAlreadyApplied, actual.AssumeAlreadyApplied, messageAndArgs...)
		assert.Equal(t, expected.FlagExcluded, actual.FlagExcluded, messageAndArgs...)

		assertOptionsEqual(t, *expected.TerragruntOptions, *actual.TerragruntOptions, messageAndArgs...)
		assertModuleListsEqual(t, expected.Dependencies, actual.Dependencies, messageAndArgs...)
	}
}

// We can't use assert.Equals on Unit or any data structure that contains it (e.g. configstack.DependencyController) because it
// contains some fields (e.g. TerragruntOptions) that cannot be compared directly
func assertDependencyControllerMapsEqual(t *testing.T, expectedModules map[string]*configstack.DependencyController, actualModules map[string]*configstack.DependencyController, doDeepCheck bool, messageAndArgs ...any) {
	t.Helper()

	if !assert.Len(t, actualModules, len(expectedModules), messageAndArgs...) {
		t.Logf("%v != %v", expectedModules, actualModules)
		return
	}

	for expectedPath, expectedModule := range expectedModules {
		actualModule, containsModule := actualModules[expectedPath]
		if assert.True(t, containsModule, messageAndArgs...) {
			assertDependencyControllersEqual(t, expectedModule, actualModule, doDeepCheck, messageAndArgs...)
		}
	}
}

// We can't use assert.Equals on Unit or any data structure that contains it (e.g. configstack.DependencyController) because it
// contains some fields (e.g. TerragruntOptions) that cannot be compared directly
func assertDependencyControllerListsEqual(t *testing.T, expectedModules []*configstack.DependencyController, actualModules []*configstack.DependencyController, doDeepCheck bool, messageAndArgs ...any) {
	t.Helper()

	if !assert.Len(t, actualModules, len(expectedModules), messageAndArgs...) {
		t.Logf("%v != %v", expectedModules, actualModules)
		return
	}

	// Build a map from path to actual controller for fast lookup
	actualByPath := map[string]*configstack.DependencyController{}
	for _, actual := range actualModules {
		actualByPath[actual.Runner.Module.Path] = actual
	}

	for _, expected := range expectedModules {
		actual, ok := actualByPath[expected.Runner.Module.Path]
		if assert.True(t, ok, messageAndArgs...) {
			assertDependencyControllersEqual(t, expected, actual, doDeepCheck, messageAndArgs...)
		}
	}
}

// We can't use assert.Equals on Unit or any data structure that contains it (e.g. configstack.DependencyController) because it
// contains some fields (e.g. TerragruntOptions) that cannot be compared directly
func assertDependencyControllersEqual(t *testing.T, expected *configstack.DependencyController, actual *configstack.DependencyController, doDeepCheck bool, messageAndArgs ...any) {
	t.Helper()

	if assert.NotNil(t, actual, messageAndArgs...) {
		assert.Equal(t, expected.Runner.Status, actual.Runner.Status, messageAndArgs...)

		assertModulesEqual(t, expected.Runner.Module, actual.Runner.Module, messageAndArgs...)
		assertErrorsEqual(t, expected.Runner.Err, actual.Runner.Err, messageAndArgs...)

		// This ensures we don't end up in a circular loop, since there is a (intentional) circular dependency
		// between NotifyWhenDone and Dependencies
		if doDeepCheck {
			assertDependencyControllerMapsEqual(t, expected.Dependencies, actual.Dependencies, false, messageAndArgs...)
			assertDependencyControllerListsEqual(t, expected.NotifyWhenDone, actual.NotifyWhenDone, false, messageAndArgs...)
		}
	}
}

// We can't do a simple IsError comparison for configstack.UnrecognizedDependencyError because that error is a struct that
// contains an array, and in Go, trying to compare arrays gives a "comparing uncomparable type
// configstack.configstack.UnrecognizedDependencyError" panic. Therefore, we have to compare that error more manually.
func assertErrorsEqual(t *testing.T, expected error, actual error, messageAndArgs ...any) {
	t.Helper()

	actual = errors.Unwrap(actual)

	var unrecognizedDependencyError common.UnrecognizedDependencyError
	if ok := errors.As(expected, &unrecognizedDependencyError); ok {
		var actualUnrecognized common.UnrecognizedDependencyError
		ok = errors.As(actual, &actualUnrecognized)
		if assert.True(t, ok, messageAndArgs...) {
			assert.Equal(t, unrecognizedDependencyError, actualUnrecognized, messageAndArgs...)
		}
	} else {
		assert.True(t, errors.IsError(actual, expected), messageAndArgs...)
	}
}

// We can't do a direct comparison between TerragruntOptions objects because we can't compare Logger or runTerragrunt
// instances. Therefore, we have to manually check everything else.
func assertOptionsEqual(t *testing.T, expected options.TerragruntOptions, actual options.TerragruntOptions, messageAndArgs ...any) {
	t.Helper()

	assert.Equal(t, expected.TerragruntConfigPath, actual.TerragruntConfigPath, messageAndArgs...)
	assert.Equal(t, expected.NonInteractive, actual.NonInteractive, messageAndArgs...)
	assert.Equal(t, expected.TerraformCliArgs, actual.TerraformCliArgs, messageAndArgs...)
	assert.Equal(t, expected.WorkingDir, actual.WorkingDir, messageAndArgs...)
}

// Return the absolute path for the given path
func canonical(t *testing.T, path string) string {
	t.Helper()

	out, err := util.CanonicalPath(path, ".")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func globCanonical(t *testing.T, path string) []string {
	t.Helper()

	out, err := util.GlobCanonicalPath(path, ".")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Create a mock TerragruntOptions object and configure its runTerragrunt command to return the given error object. If
// the runTerragrunt command is called, this method will also set the executed boolean to true.
func optionsWithMockTerragruntCommand(t *testing.T, terragruntConfigPath string, toReturnFromTerragruntCommand error, executed *bool) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(terragruntConfigPath)
	if err != nil {
		t.Fatalf("Error creating terragrunt options for test %v", err)
	}
	opts.RunTerragrunt = func(_ context.Context, _ log.Logger, _ *options.TerragruntOptions, _ *report.Report) error {
		*executed = true
		return toReturnFromTerragruntCommand
	}
	return opts
}

func assertMultiErrorContains(t *testing.T, actualError error, expectedErrors ...error) {
	t.Helper()

	multiError := new(errors.MultiError)
	errors.As(actualError, &multiError)
	require.NotNil(t, multiError, "Expected a MutliError, but got: %v", actualError)

	assert.Len(t, multiError.WrappedErrors(), len(expectedErrors))
	for _, expectedErr := range expectedErrors {
		found := false
		for _, actualErr := range multiError.WrappedErrors() {
			if errors.Is(expectedErr, actualErr) {
				found = true

				break
			}
		}
		assert.True(t, found, "Couldn't find expected error %v", expectedErr)
	}
}

func ptr(str string) *string {
	return &str
}
