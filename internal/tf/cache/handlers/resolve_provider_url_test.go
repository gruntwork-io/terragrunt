package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProviderURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		providersV1  string
		registryName string
		pathParts    []string
		expected     string
	}{
		{
			name:         "relative path builds https URL with registry host",
			providersV1:  "/v1/providers",
			registryName: "registry.example.com",
			pathParts:    []string{"hashicorp", "aws", "versions"},
			expected:     "https://registry.example.com/v1/providers/hashicorp/aws/versions",
		},
		{
			name:         "relative path with no leading slash",
			providersV1:  "v1/providers",
			registryName: "registry.example.com",
			pathParts:    []string{"hashicorp", "aws", "versions"},
			expected:     "https://registry.example.com/v1/providers/hashicorp/aws/versions",
		},
		{
			name:         "absolute https URL uses it as base",
			providersV1:  "https://nexus.corp.com/repository/terraform/v1/providers",
			registryName: "registry.example.com",
			pathParts:    []string{"hashicorp", "aws", "versions"},
			expected:     "https://nexus.corp.com/repository/terraform/v1/providers/hashicorp/aws/versions",
		},
		{
			name:         "absolute https URL with trailing slash strips it",
			providersV1:  "https://nexus.corp.com/v1/providers/",
			registryName: "registry.example.com",
			pathParts:    []string{"hashicorp", "aws"},
			expected:     "https://nexus.corp.com/v1/providers/hashicorp/aws",
		},
		{
			name:         "absolute http URL uses it as base",
			providersV1:  "http://internal.registry.local/providers",
			registryName: "registry.example.com",
			pathParts:    []string{"myns", "myprovider", "versions"},
			expected:     "http://internal.registry.local/providers/myns/myprovider/versions",
		},
		{
			name:         "no path parts returns base URL",
			providersV1:  "https://nexus.corp.com/v1/providers",
			registryName: "registry.example.com",
			pathParts:    []string{},
			expected:     "https://nexus.corp.com/v1/providers",
		},
		{
			name:         "relative path with no path parts",
			providersV1:  "/v1/providers",
			registryName: "registry.example.com",
			pathParts:    []string{},
			expected:     "https://registry.example.com/v1/providers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := ResolveProviderURL(tt.providersV1, tt.registryName, tt.pathParts...)
			require.NotNil(t, result)
			assert.Equal(t, tt.expected, result.String())
		})
	}
}
