package test_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureLocalsErrorUndefinedLocal         = "fixtures/locals-errors/undefined-local"
	testFixtureLocalsErrorUndefinedLocalButInput = "fixtures/locals-errors/undefined-local-but-input"
	testFixtureLocalsCanonical                   = "fixtures/locals/canonical"
	testFixtureLocalsInInclude                   = "fixtures/locals/local-in-include"
	testFixtureLocalRunOnce                      = "fixtures/locals/run-once"
	testFixtureLocalRunMultiple                  = "fixtures/locals/run-multiple"
	testFixtureLocalsInIncludeChildRelPath       = "qa/my-app"
	testFixtureBrokenLocals                      = "fixtures/broken-locals"
)

func TestUndefinedLocalsReferenceBreaks(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLocalsErrorUndefinedLocal)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLocalsErrorUndefinedLocal)
	helpers.CleanupTerraformFolder(t, rootPath)
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		os.Stdout,
		os.Stderr,
	)
	require.Error(t, err)
}

func TestUndefinedLocalsReferenceToInputsBreaks(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLocalsErrorUndefinedLocalButInput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLocalsErrorUndefinedLocalButInput)
	helpers.CleanupTerraformFolder(t, rootPath)
	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath,
		os.Stdout,
		os.Stderr,
	)
	require.Error(t, err)
}

func TestLogFailedLocalsEvaluation(t *testing.T) {
	t.Parallel()

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	err := helpers.RunTerragruntCommand(
		t,
		fmt.Sprintf(
			"terragrunt apply -auto-approve --non-interactive --working-dir %s --log-level trace",
			testFixtureBrokenLocals,
		),
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	output := stderr.String()
	assert.Contains(
		t,
		output,
		"Encountered error while evaluating locals in file "+filepath.FromSlash("./terragrunt.hcl"),
	)
}

func TestTerragruntLocalRunOnce(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLocalRunOnce)
	rootPath := filepath.Join(tmpEnvPath, testFixtureLocalRunOnce)
	helpers.CleanupTerraformFolder(t, rootPath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt init --working-dir "+rootPath,
		&stdout,
		&stderr,
	)
	require.Error(t, err)

	errout := stdout.String()

	assert.Equal(t, 1, strings.Count(errout, "foo"))
}
