package spin

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/gruntwork-io/terragrunt/options"
	"path/filepath"
	"github.com/gruntwork-io/terragrunt/locks/dynamodb"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/gruntwork-io/terragrunt/locks"
	"github.com/gruntwork-io/terragrunt/errors"
	"sort"
)

type TerraformModuleByPath []*TerraformModule
func (byPath TerraformModuleByPath) Len() int           { return len(byPath) }
func (byPath TerraformModuleByPath) Swap(i, j int)      { byPath[i], byPath[j] = byPath[j], byPath[i] }
func (byPath TerraformModuleByPath) Less(i, j int) bool { return byPath[i].Path < byPath[j].Path }

type RunningModuleByPath []*runningModule
func (byPath RunningModuleByPath) Len() int           { return len(byPath) }
func (byPath RunningModuleByPath) Swap(i, j int)      { byPath[i], byPath[j] = byPath[j], byPath[i] }
func (byPath RunningModuleByPath) Less(i, j int) bool { return byPath[i].Module.Path < byPath[j].Module.Path }

// We can't use assert.Equals on TerraformModule or any data structure that contains it because it contains some
// fields (e.g. TerragruntOptions) that cannot be compared directly
func assertModuleListsEqual(t *testing.T, expectedModules []*TerraformModule, actualModules []*TerraformModule, messageAndArgs ...interface{}) {
	if !assert.Equal(t, len(expectedModules), len(actualModules), messageAndArgs...) {
		t.Logf("%s != %s", expectedModules, actualModules)
		return
	}

	sort.Sort(TerraformModuleByPath(expectedModules))
	sort.Sort(TerraformModuleByPath(actualModules))

	for i := 0; i < len(expectedModules); i++ {
		expected := expectedModules[i]
		actual := actualModules[i]
		assertModulesEqual(t, expected, actual, messageAndArgs...)
	}
}

// We can't use assert.Equals on TerraformModule because it contains some fields (e.g. TerragruntOptions) that cannot
// be compared directly
func assertModulesEqual(t *testing.T, expected *TerraformModule, actual *TerraformModule, messageAndArgs ...interface{}) {
	if assert.NotNil(t, actual, messageAndArgs...) {
		assert.Equal(t, expected.Config, actual.Config, messageAndArgs...)
		assert.Equal(t, expected.Path, actual.Path, messageAndArgs...)

		assertOptionsEqual(t, *expected.TerragruntOptions, *actual.TerragruntOptions, messageAndArgs...)
		assertModuleListsEqual(t, expected.Dependencies, actual.Dependencies, messageAndArgs...)
	}
}

// We can't use assert.Equals on TerraformModule or any data structure that contains it (e.g. runningModule) because it
// contains some fields (e.g. TerragruntOptions) that cannot be compared directly
func assertRunningModuleMapsEqual(t *testing.T, expectedModules map[string]*runningModule, actualModules map[string]*runningModule, doDeepCheck bool, messageAndArgs ...interface{}) {
	if !assert.Equal(t, len(expectedModules), len(actualModules), messageAndArgs...) {
		t.Logf("%s != %s", expectedModules, actualModules)
		return
	}

	for expectedPath, expectedModule := range expectedModules {
		actualModule, containsModule := actualModules[expectedPath]
		if assert.True(t, containsModule, messageAndArgs...) {
			assertRunningModulesEqual(t, expectedModule, actualModule, doDeepCheck, messageAndArgs...)
		}
	}
}

// We can't use assert.Equals on TerraformModule or any data structure that contains it (e.g. runningModule) because it
// contains some fields (e.g. TerragruntOptions) that cannot be compared directly
func assertRunningModuleListsEqual(t *testing.T, expectedModules []*runningModule, actualModules []*runningModule, doDeepCheck bool, messageAndArgs ...interface{}) {
	if !assert.Equal(t, len(expectedModules), len(actualModules), messageAndArgs...) {
		t.Logf("%s != %s", expectedModules, actualModules)
		return
	}

	sort.Sort(RunningModuleByPath(expectedModules))
	sort.Sort(RunningModuleByPath(actualModules))

	for i := 0; i < len(expectedModules); i++ {
		expected := expectedModules[i]
		actual := actualModules[i]
		assertRunningModulesEqual(t, expected, actual, doDeepCheck, messageAndArgs...)
	}
}

// We can't use assert.Equals on TerraformModule or any data structure that contains it (e.g. runningModule) because it
// contains some fields (e.g. TerragruntOptions) that cannot be compared directly
func assertRunningModulesEqual(t *testing.T, expected *runningModule, actual *runningModule, doDeepCheck bool, messageAndArgs ...interface{}) {
	if assert.NotNil(t, actual, messageAndArgs...) {
		assert.Equal(t, expected.Status, actual.Status, messageAndArgs...)

		assertModulesEqual(t, expected.Module, actual.Module, messageAndArgs...)
		assertErrorsEqual(t, expected.Err, actual.Err, messageAndArgs...)

		// This ensures we don't end up in a circular loop, since there is a (intentional) circular dependency
		// between NotifyWhenDone and Dependencies
		if doDeepCheck {
			assertRunningModuleMapsEqual(t, expected.Dependencies, actual.Dependencies, false, messageAndArgs...)
			assertRunningModuleListsEqual(t, expected.NotifyWhenDone, actual.NotifyWhenDone, false, messageAndArgs...)
		}
	}
}

// We can't do a simple IsError comparison for UnrecognizedDependency because that error is a struct that
// contains an array, and in Go, trying to compare arrays gives a "comparing uncomparable type
// spin.UnrecognizedDependency" panic. Therefore, we have to compare that error more manually.
func assertErrorsEqual(t *testing.T, expected error, actual error, messageAndArgs ...interface{}) {
	actual = errors.Unwrap(actual)
	if expectedUnrecognized, isUnrecognizedDependencyError := expected.(UnrecognizedDependency); isUnrecognizedDependencyError {
		actualUnrecognized, isUnrecognizedDependencyError := actual.(UnrecognizedDependency)
		if assert.True(t, isUnrecognizedDependencyError, messageAndArgs...) {
			assert.Equal(t, expectedUnrecognized, actualUnrecognized, messageAndArgs...)
		}
	} else {
		assert.True(t, errors.IsError(actual, expected), messageAndArgs...)
	}
}

// We can't do a direct comparison between TerragruntOptions objects because we can't compare Logger or RunTerragrunt
// instances. Therefore, we have to manually check everything else.
func assertOptionsEqual(t *testing.T, expected options.TerragruntOptions, actual options.TerragruntOptions, messageAndArgs ...interface{}) {
	assert.NotNil(t, expected.Logger, messageAndArgs...)
	assert.NotNil(t, actual.Logger, messageAndArgs...)

	assert.Equal(t, expected.TerragruntConfigPath, actual.TerragruntConfigPath, messageAndArgs...)
	assert.Equal(t, expected.NonInteractive, actual.NonInteractive, messageAndArgs...)
	assert.Equal(t, expected.TerraformCliArgs, actual.TerraformCliArgs, messageAndArgs...)
	assert.Equal(t, expected.WorkingDir, actual.WorkingDir, messageAndArgs...)
}

// Return the absolute path for the given path
func abs(t *testing.T, path string) string {
	out, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Create a new DynamoDB lock
func lock(t *testing.T, stateFileId string) locks.Lock {
	lock, err := dynamodb.New(map[string]string{"state_file_id": stateFileId})
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

// Create a RemoteState struct
func state(t *testing.T, bucket string, key string) *remote.RemoteState {
	return &remote.RemoteState{
		Backend: "s3",
		Config: map[string]string{
			"bucket": bucket,
			"key": key,
		},
	}
}