//go:build gcp

package test_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureOverviewGCP = "fixtures/docs/02-overview-gcp"

// TestGCPDocsOverview applies and destroys each step of the GCP tab of the
// Getting Started overview, docs/src/content/docs/01-getting-started/02-overview.mdx.
// It does for GCP what TestAWSDocsOverview does for AWS. Each step bootstraps
// its own state bucket, so the steps run in parallel.
func TestGCPDocsOverview(t *testing.T) {
	t.Parallel()

	steps := []struct {
		// name is the fixture directory of the step.
		name string
		// mockNetworks lists the mock network names the plan output must
		// contain, one per region, so every region's router is verified.
		mockNetworks []string
		// singleUnit marks the one-unit step, which runs without --all.
		singleUnit bool
	}{
		{name: "step-01-terragrunt.hcl", singleUnit: true},
		{name: "step-02-dependencies"},
		{name: "step-03-mock-outputs", mockNetworks: []string{`"mock-network"`}},
		{name: "step-04-configuration-hierarchy", mockNetworks: []string{`"mock-network"`}},
		{
			name:         "step-05-exposed-includes",
			mockNetworks: []string{`"mock-network-us-central1"`, `"mock-network-us-east1"`},
		},
	}

	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			t.Parallel()

			rootPath, bucket := setupGCPDocsStep(t, step.name)

			applyCmd := "terragrunt run --all --non-interactive --backend-bootstrap --working-dir " +
				rootPath + " -- apply -auto-approve"
			destroyCmd := "terragrunt run --all --non-interactive --working-dir " +
				rootPath + " -- destroy -auto-approve"

			if step.singleUnit {
				applyCmd = "terragrunt run --non-interactive --backend-bootstrap --working-dir " +
					rootPath + " -- apply -auto-approve"
				destroyCmd = "terragrunt destroy -auto-approve --non-interactive --working-dir " + rootPath
			}

			defer deleteGCSBucket(t, bucket)

			// A failed destroy must not hide the apply error that caused it.
			defer func() {
				_, _, err := helpers.RunTerragruntCommandWithOutput(t, destroyCmd)
				assert.NoError(t, err, "destroying the step's infrastructure")
			}()

			if len(step.mockNetworks) > 0 {
				planCmd := "terragrunt run --all --non-interactive --backend-bootstrap --working-dir " +
					rootPath + " -- plan"

				stdout, _, err := helpers.RunTerragruntCommandWithOutput(t, planCmd)
				require.NoError(t, err)

				for _, network := range step.mockNetworks {
					require.Contains(t, stdout, network,
						"every region's router plan must use its mock network while its vpc has no outputs")
				}
			}

			_, _, err := helpers.RunTerragruntCommandWithOutput(t, applyCmd)
			require.NoError(t, err)
		})
	}
}

// setupGCPDocsStep copies one fixture step and points it at a state bucket
// that does not exist yet. It returns the step's root path and the bucket
// name. The caller deletes the bucket after the step's destroy.
func setupGCPDocsStep(t *testing.T, step string) (string, string) {
	t.Helper()

	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	require.NotEmpty(t, project, "GOOGLE_CLOUD_PROJECT must name the project the docs steps run in")

	id := strings.ToLower(helpers.UniqueID())
	bucket := "tg-docs-state-" + id

	stepPath := filepath.Join(testFixtureOverviewGCP, step)
	helpers.CleanupTerraformFolder(t, stepPath)
	tmpEnvPath := helpers.CopyEnvironment(t, stepPath)
	rootPath := filepath.Join(tmpEnvPath, stepPath)

	// Network names must be unique within a project, and subnet names within a
	// region of it. The steps share one project, so the prefix keeps them apart.
	placeholders := map[string]string{
		"__FILL_IN_PROJECT_ID__":  project,
		"__FILL_IN_BUCKET_NAME__": bucket,
		"__FILL_IN_NAME_PREFIX__": "tg-docs-" + id,
	}

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Lock files end in .hcl too, but hold no placeholders, so leave them alone.
		if d.IsDir() || filepath.Ext(path) != ".hcl" || d.Name() == ".terraform.lock.hcl" {
			return nil
		}

		helpers.CopyAndFillMapPlaceholders(t, path, path, placeholders)

		return nil
	})
	require.NoError(t, err)

	return rootPath, bucket
}
