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

	testCases := []struct {
		config   backend.Config
		expected map[string]any
		name     string
	}{
		{
			name:     "empty-no-values",
			config:   backend.Config{},
			expected: map[string]any{},
		},
		{
			name: "valid-gcs-configuration-keys",
			config: backend.Config{
				"bucket":      "my-bucket",
				"prefix":      "terraform/state",
				"credentials": "path/to/creds.json",
			},
			expected: map[string]any{
				"bucket":      "my-bucket",
				"prefix":      "terraform/state",
				"credentials": "path/to/creds.json",
			},
		},
		{
			name: "terragrunt-keys-filtered",
			config: backend.Config{
				"bucket":                    "my-bucket",
				"prefix":                    "terraform/state",
				"project":                   "my-project",
				"location":                  "us-central1",
				"gcs_bucket_labels":         map[string]string{"env": "prod"},
				"skip_bucket_versioning":    true,
				"skip_bucket_creation":      true,
				"enable_bucket_policy_only": true,
			},
			expected: map[string]any{
				"bucket": "my-bucket",
				"prefix": "terraform/state",
			},
		},
		{
			name: "empty-after-all-terragrunt-keys-filtered",
			config: backend.Config{
				"project":                   "my-project",
				"location":                  "us-central1",
				"gcs_bucket_labels":         map[string]string{},
				"skip_bucket_versioning":    true,
				"skip_bucket_creation":      false,
				"enable_bucket_policy_only": false,
			},
			expected: map[string]any{},
		},
		{
			name: "string-bool-normalization-passthrough",
			config: backend.Config{
				"bucket": "my-bucket",
				"prefix": "terraform/state",
			},
			expected: map[string]any{
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
