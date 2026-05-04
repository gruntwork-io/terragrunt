package exec_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnvSliceFromMap pins the contract: length matches input, output is sorted,
// every k=v pair from the input is present, and the split point is the FIRST '='.
func TestEnvSliceFromMap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		in   map[string]string
		name string
		want []string
	}{
		{name: "nil", in: nil, want: []string{}},
		{name: "empty", in: map[string]string{}, want: []string{}},
		{name: "single", in: map[string]string{"FOO": "bar"}, want: []string{"FOO=bar"}},
		{
			name: "multiple-overlapping-prefixes",
			in:   map[string]string{"FOOBAR": "1", "FOO": "0", "FOOZ": "2"},
			want: []string{"FOO=0", "FOOBAR=1", "FOOZ=2"},
		},
		{
			name: "value-contains-equals",
			in:   map[string]string{"PATH": "/a=b", "X": "1"},
			want: []string{"PATH=/a=b", "X=1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := exec.EnvSliceFromMap(tc.in)

			assert.Len(t, got, len(tc.in), "length must match input map")
			assert.True(t, slices.IsSorted(got), "output must be sorted; got %v", got)
			assert.ElementsMatch(t, tc.want, got, "multiset of k=v entries must match input")

			for _, e := range got {
				k, v, ok := strings.Cut(e, "=")
				require.True(t, ok, "expected k=v form, got %q", e)
				require.Equal(t, tc.in[k], v, "value mismatch for key %q", k)
			}
		})
	}
}
