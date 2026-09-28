package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestColorTableIsCurrent pins the committed table against what lipgloss renders
// now. A lipgloss upgrade without a regeneration diverges here rather than in a
// user's terminal.
func TestColorTableIsCurrent(t *testing.T) {
	t.Parallel()

	for value, want := range options.BuildColorTable() {
		if want.Prefix == "" && want.Suffix == "" {
			continue
		}

		got, err := options.Color(options.ColorValue(value)).Format(&options.Data{}, "x")
		require.NoError(t, err)
		assert.Equal(
			t,
			want.Prefix+"x"+want.Suffix,
			got,
			"color %d is stale; run `go generate ./pkg/log/format/options/...`",
			value,
		)
	}
}
