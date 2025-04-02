//go:build private_registry

package test_test

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

const (
	privateRegistryFixturePath = "fixtures/private-registry"
)

func setupPrivateRegistryTest(t *testing.T) (string, string, string) {
	t.Helper()

	registryToken := os.Getenv("PRIVATE_REGISTRY_TOKEN")

	// the private registry test is recommended to be a clone of gruntwork-io/terraform-null-terragrunt-registry-test
	registryUrl := os.Getenv("PRIVATE_REGISTRY_URL")

	if registryToken == "" || registryUrl == "" {
		t.Skip("Skipping test because it requires a valid Terraform registry token and url")
	}

	helpers.CleanupTerraformFolder(t, privateRegistryFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, privateRegistryFixturePath)
	rootPath := util.JoinPath(tmpEnvPath, privateRegistryFixturePath)

	URL, err := url.Parse("tfr://" + registryUrl)
	if err != nil {
		t.Fatalf("REGISTRY_URL is invalid: %v", err)
	}

	if URL.Hostname() == "" {
		t.Fatal("REGISTRY_URL is invalid")
	}

	helpers.CopyAndFillMapPlaceholders(t, util.JoinPath(privateRegistryFixturePath, "terragrunt.hcl"), util.JoinPath(rootPath, "terragrunt.hcl"), map[string]string{
		"__registry_url__": registryUrl,
	})

	return rootPath, URL.Hostname(), registryToken
}

func TestPrivateRegistryWithConfgFileToken(t *testing.T) {
	rootPath, host, token := setupPrivateRegistryTest(t)

	helpers.CopyAndFillMapPlaceholders(t, util.JoinPath(privateRegistryFixturePath, "env.tfrc"), util.JoinPath(rootPath, "env.tfrc"), map[string]string{
		"__registry_token__": token,
		"__registry_host__":  host,
	})

	t.Setenv("TF_CLI_CONFIG_FILE", util.JoinPath(rootPath, "env.tfrc"))

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt init --non-interactive --log-level=trace --working-dir="+rootPath)

	// the hashicorp/null provider errors on install, but that indicates that the private tfr module was downloaded
	require.Contains(t, err.Error(), "hashicorp/null", "Error accessing the private registry")
}

func TestPrivateRegistryWithEnvToken(t *testing.T) {
	rootPath, host, token := setupPrivateRegistryTest(t)

	// Convert host to format suitable for Terraform env vars.
	// This is based on the tf/cliconfig/credentials.go collectCredentialsFromEnv
	host = strings.ReplaceAll(strings.ReplaceAll(host, ".", "_"), "-", "__")

	t.Setenv(fmt.Sprintf("TF_TOKEN_%s", host), token)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt init --non-interactive --log-level=trace --working-dir="+rootPath)

	// The main test is for authentication against the private registry, so if the null provider fails then we know
	// that terragrunt authenticated and downloaded the module.
	require.Contains(t, err.Error(), "hashicorp/null", "Error accessing the private registry")
}
