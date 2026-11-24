package diagnostic_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/stretchr/testify/assert"
)

// TestRangeString pins the short form of a range on one line and the long form of a range over several.
func TestRangeString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		expected string
		rng      diagnostic.Range
	}{
		{
			name: "one line",
			rng: diagnostic.Range{
				Filename: "terragrunt.hcl",
				Start:    diagnostic.Pos{Line: 2, Column: 3, Byte: 12},
				End:      diagnostic.Pos{Line: 2, Column: 9, Byte: 18},
			},
			expected: "terragrunt.hcl:2,3-9",
		},
		{
			name: "several lines",
			rng: diagnostic.Range{
				Filename: "terragrunt.hcl",
				Start:    diagnostic.Pos{Line: 2, Column: 3, Byte: 12},
				End:      diagnostic.Pos{Line: 4, Column: 1, Byte: 30},
			},
			expected: "terragrunt.hcl:2,3-4,1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, tc.rng.String())
		})
	}
}
