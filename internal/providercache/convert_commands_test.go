package providercache_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/providercache"
	"github.com/stretchr/testify/assert"
)

func TestConvertToMultipleCommandsByPlatforms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		expected [][]string
	}{
		{
			name: "equals form is split one platform per command",
			args: []string{"providers", "lock", "-platform=linux_amd64", "-platform=darwin_arm64"},
			expected: [][]string{
				{"providers", "lock", "-platform=linux_amd64"},
				{"providers", "lock", "-platform=darwin_arm64"},
			},
		},
		{
			name: "space separated form is normalized and split",
			args: []string{"providers", "lock", "-platform", "linux_amd64", "-platform", "darwin_arm64"},
			expected: [][]string{
				{"providers", "lock", "-platform=linux_amd64"},
				{"providers", "lock", "-platform=darwin_arm64"},
			},
		},
		{
			name: "both forms in the same command",
			args: []string{"providers", "lock", "-platform", "linux_amd64", "-platform=darwin_arm64"},
			expected: [][]string{
				{"providers", "lock", "-platform=linux_amd64"},
				{"providers", "lock", "-platform=darwin_arm64"},
			},
		},
		{
			name: "single space separated platform",
			args: []string{"providers", "lock", "-platform", "windows_amd64"},
			expected: [][]string{
				{"providers", "lock", "-platform=windows_amd64"},
			},
		},
		{
			name: "other flags and provider addresses are kept in every command",
			args: []string{"providers", "lock", "-fs-mirror=/tmp/mirror", "-platform", "linux_amd64", "hashicorp/aws", "-platform=darwin_arm64"},
			expected: [][]string{
				{"providers", "lock", "-fs-mirror=/tmp/mirror", "hashicorp/aws", "-platform=linux_amd64"},
				{"providers", "lock", "-fs-mirror=/tmp/mirror", "hashicorp/aws", "-platform=darwin_arm64"},
			},
		},
		{
			name: "no platform flag returns the command unchanged",
			args: []string{"providers", "lock", "hashicorp/aws"},
			expected: [][]string{
				{"providers", "lock", "hashicorp/aws"},
			},
		},
		{
			name: "trailing platform flag without a value is left for OpenTofu to reject",
			args: []string{"providers", "lock", "-platform"},
			expected: [][]string{
				{"providers", "lock", "-platform"},
			},
		},
		{
			name: "double dash form is left untouched",
			args: []string{"providers", "lock", "--platform", "linux_amd64"},
			expected: [][]string{
				{"providers", "lock", "--platform", "linux_amd64"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := make([]string, len(tt.args))
			copy(args, tt.args)

			got := providercache.ConvertToMultipleCommandsByPlatforms(args)

			assert.Equal(t, tt.expected, got)
			assert.Equal(t, tt.args, args, "the given arguments must not be modified")
		})
	}
}
