//go:build private_registry

package test_test

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/require"
)

const (
	privateRegistryFixturePath = "fixtures/private-registry"
)

func setupPrivateRegistryTest(t *testing.T) (string, string, string) {
	t.Helper()

	registryToken := os.Getenv("PRIVATE_REGISTRY_TOKEN")

	// the private registry test is recommended to be a clone of gruntwork-io/terraform-null-terragrunt-registry-test
	registryURL := os.Getenv("PRIVATE_REGISTRY_URL")

	if registryToken == "" || registryURL == "" {
		t.Skip("Skipping test because it requires a valid Terraform registry token and url")
	}

	helpers.CleanupTerraformFolder(t, privateRegistryFixturePath)
	tmpEnvPath := helpers.CopyEnvironment(t, privateRegistryFixturePath)
	rootPath := filepath.Join(tmpEnvPath, privateRegistryFixturePath)

	URL, err := url.Parse("tfr://" + registryURL)
	if err != nil {
		t.Fatalf("REGISTRY_URL is invalid: %v", err)
	}

	if URL.Hostname() == "" {
		t.Fatal("REGISTRY_URL is invalid")
	}

	helpers.CopyAndFillMapPlaceholders(t, filepath.Join(privateRegistryFixturePath, "terragrunt.hcl"), filepath.Join(rootPath, "terragrunt.hcl"), map[string]string{
		"__registry_url__": registryURL,
	})

	return rootPath, URL.Hostname(), registryToken
}

func TestPrivateRegistryWithConfgFileToken(t *testing.T) {
	rootPath, host, token := setupPrivateRegistryTest(t)

	helpers.CopyAndFillMapPlaceholders(t, filepath.Join(privateRegistryFixturePath, "env.tfrc"), filepath.Join(rootPath, "env.tfrc"), map[string]string{
		"__registry_token__": token,
		"__registry_host__":  host,
	})

	t.Setenv("TF_CLI_CONFIG_FILE", filepath.Join(rootPath, "env.tfrc"))

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt init --non-interactive --log-level=trace --working-dir="+rootPath)

	// the hashicorp/null provider errors on install, but that indicates that the private tfr module was downloaded
	require.Contains(t, err.Error(), "hashicorp/null", "Error accessing the private registry")
}

// TestPrivateRegistrySubDomainWithConfigFileToken verifies that a credentials block configured for a parent
// domain (e.g. "example.com") is used to authenticate requests to a subdomain of that domain (e.g.
// "app.example.com"). This mirrors registries like Spacelift, where you authenticate against "spacelift.io"
// but modules are downloaded from "app.spacelift.io". For this test to exercise that path, PRIVATE_REGISTRY_URL
// must point at a host with a subdomain (at least three labels, e.g. "app.example.com").
func TestPrivateRegistrySubDomainWithConfigFileToken(t *testing.T) {
	rootPath, host, token := setupPrivateRegistryTest(t)

	hostParts := strings.Split(host, ".")
	if len(hostParts) < 3 {
		t.Skipf("Skipping test because PRIVATE_REGISTRY_URL host %q has no subdomain to test suffix matching against; use a host like \"app.example.com\"", host)
	}

	// Strip the leftmost label so the credentials block is configured for the parent domain rather than the
	// exact host used to download the module. Terragrunt should still find these credentials via the
	// ForHostSuffix fallback.
	parentHost := strings.Join(hostParts[1:], ".")

	helpers.CopyAndFillMapPlaceholders(t, filepath.Join(privateRegistryFixturePath, "env.tfrc"), filepath.Join(rootPath, "env.tfrc"), map[string]string{
		"__registry_token__": token,
		"__registry_host__":  parentHost,
	})

	t.Setenv("TF_CLI_CONFIG_FILE", filepath.Join(rootPath, "env.tfrc"))

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt init --non-interactive --log-level=trace --working-dir="+rootPath)

	// the hashicorp/null provider errors on install, but that indicates that the private tfr module was downloaded
	// using credentials matched by suffix against the parent domain, not an exact host match.
	require.Contains(t, err.Error(), "hashicorp/null", "Error accessing the private registry")
}

func TestPrivateRegistryWithEnvToken(t *testing.T) {
	rootPath, host, token := setupPrivateRegistryTest(t)

	// Convert host to format suitable for Terraform env vars.
	// This is based on the tf/cliconfig/credentials.go collectCredentialsFromEnv
	host = strings.ReplaceAll(strings.ReplaceAll(host, ".", "_"), "-", "__")

	t.Setenv("TF_TOKEN_"+host, token)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt init --non-interactive --log-level=trace --working-dir="+rootPath)

	// The main test is for authentication against the private registry, so if the null provider fails then we know
	// that terragrunt authenticated and downloaded the module.
	require.Contains(t, err.Error(), "hashicorp/null", "Error accessing the private registry")
}
