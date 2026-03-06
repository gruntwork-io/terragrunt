package gcs_test

import (
	"testing"

	backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	gcsbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"

	"github.com/stretchr/testify/assert"
)

func TestBackend_GetTFInitArgs(t *testing.T) {
	t.Parallel()

	remoteBackend := gcsbackend.NewBackend()

	testCases := []struct { //nolint: govet
		name     string
		config   backend.Config
		expected map[string]any
	}{
		{
			"empty-no-values",
			backend.Config{},
			map[string]any{},
		},
		{
			"valid-gcs-configuration-keys",
			backend.Config{
				"bucket":      "my-bucket",
				"prefix":      "terraform/state",
				"credentials": "path/to/creds.json",
			},
			map[string]any{
				"bucket":      "my-bucket",
				"prefix":      "terraform/state",
				"credentials": "path/to/creds.json",
			},
		},
		{
			"terragrunt-keys-filtered",
			backend.Config{
				"bucket":                    "my-bucket",
				"prefix":                    "terraform/state",
				"project":                   "my-project",
				"location":                  "us-central1",
				"gcs_bucket_labels":         map[string]string{"env": "prod"},
				"skip_bucket_versioning":    true,
				"skip_bucket_creation":      true,
				"enable_bucket_policy_only": true,
			},
			map[string]any{
				"bucket": "my-bucket",
				"prefix": "terraform/state",
			},
		},
		{
			"empty-after-all-terragrunt-keys-filtered",
			backend.Config{
				"project":                   "my-project",
				"location":                  "us-central1",
				"gcs_bucket_labels":         map[string]string{},
				"skip_bucket_versioning":    true,
				"skip_bucket_creation":      false,
				"enable_bucket_policy_only": false,
			},
			map[string]any{},
		},
		{
			"string-bool-normalization-passthrough",
			backend.Config{
				"bucket": "my-bucket",
				"prefix": "terraform/state",
			},
			map[string]any{
				"bucket": "my-bucket",
				"prefix": "terraform/state",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := remoteBackend.GetTFInitArgs(tc.config)

			assert.Equal(t, tc.expected, actual)
		})
	}
}
