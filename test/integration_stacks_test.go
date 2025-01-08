package test_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	testFixtureStacksBasic  = "fixtures/stacks/basic"
	testFixtureStacksLocals = "fixtures/stacks/locals"
	testFixtureStacksRemote = "fixtures/stacks/remote"
)

func TestStacksGenerateBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic)

	helpers.RunTerragrunt(t, "terragrunt stack generate --terragrunt-working-dir "+rootPath)
}

func TestStacksGenerateLocals(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksLocals)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksLocals)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksLocals)

	helpers.RunTerragrunt(t, "terragrunt stack generate --terragrunt-working-dir "+rootPath)
}

func TestStacksGenerateRemote(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureStacksRemote)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksRemote)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksRemote)

	helpers.RunTerragrunt(t, "terragrunt stack generate --terragrunt-working-dir "+rootPath)
}
