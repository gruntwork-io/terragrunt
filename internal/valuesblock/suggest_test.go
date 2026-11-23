package valuesblock_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHint(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		ref   string
		want  string
		names []string
	}{
		{
			name:  "close name",
			names: []string{"tags", "region"},
			ref:   "tgas",
			want:  "\n\nDid you mean \"tags\"?",
		},
		{
			name: "no names",
			ref:  "tags",
			want: "\n\nThe values block declares no values.",
		},
		{
			name:  "few names",
			names: []string{"region", "account"},
			ref:   "zzzzzz",
			want:  "\n\nDeclared values: account, region.",
		},
		{
			name:  "more names than the hint lists",
			names: []string{"v00", "v01", "v02", "v03", "v04", "v05", "v06", "v07", "v08", "v09", "v10", "v11"},
			ref:   "zzzzzz",
			want:  "\n\nDeclared values: v00, v01, v02, v03, v04, v05, v06, v07, v08, v09, and 2 more.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var src strings.Builder

			src.WriteString("values {\n")

			for _, name := range tc.names {
				fmt.Fprintf(&src, "  value %q {}\n", name)
			}

			src.WriteString("}\n")

			assert.Equal(t, tc.want, decode(t, src.String()).Hint(tc.ref))
		})
	}
}
