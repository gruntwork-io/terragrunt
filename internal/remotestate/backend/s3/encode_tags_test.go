package s3_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
)

func TestEncodeTagsForHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tags     map[string]string
		expected string
	}{
		{
			name:     "nil tags",
			tags:     nil,
			expected: "",
		},
		{
			name:     "empty tags",
			tags:     map[string]string{},
			expected: "",
		},
		{
			name:     "single tag",
			tags:     map[string]string{"env": "prod"},
			expected: "env=prod",
		},
		{
			name:     "multiple tags sorted",
			tags:     map[string]string{"env": "prod", "app": "web"},
			expected: "app=web&env=prod",
		},
		{
			name:     "tags with special characters",
			tags:     map[string]string{"team name": "my team", "cost&center": "abc=123"},
			expected: "cost%26center=abc%3D123&team+name=my+team",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := s3.EncodeTagsForHeader(tt.tags)
			assert.Equal(t, tt.expected, result)
		})
	}
}
