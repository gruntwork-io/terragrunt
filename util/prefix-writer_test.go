package util_test

import (
	"bytes"
	"errors"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrefixWriter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		prefix   string
		expected string
		values   []string
	}{
		{
			prefix:   "p1 ",
			expected: "p1 ab",
			values:   []string{"a", "b"},
		},
		{
			prefix:   "p2 ",
			expected: "p2 ab",
			values:   []string{"a", "b"},
		},
		{
			prefix:   "",
			expected: "ab",
			values:   []string{"a", "b"},
		},
		{
			prefix:   "p1 ",
			expected: "p1 ab\n",
			values:   []string{"a", "b\n"},
		},
		{
			prefix:   "p1 ",
			expected: "p1 a\np1 b",
			values:   []string{"a\n", "b"},
		},
		{
			prefix:   "p1 ",
			expected: "p1 a\np1 b\n",
			values:   []string{"a\n", "b\n"},
		},
		{
			prefix:   "p1 ",
			expected: "p1 abcdef",
			values:   []string{"a", "b", "c", "def"},
		},
		{
			prefix:   "p1 ",
			expected: "p1 abcdefgh\n",
			values:   []string{"ab", "cd", "ef", "gh\n"},
		},
		{
			prefix:   "p1 ",
			expected: "p1 abcd\np1 efgh\n",
			values:   []string{"ab", "cd\n", "ef", "gh\n"},
		},
		{
			prefix:   "p1 ",
			expected: "p1 abcde\np1 fgh\n",
			values:   []string{"ab", "cd", "e\nf", "gh\n"},
		},
		{
			prefix:   "p1 ",
			expected: "p1 abcdef\np1 gh\n",
			values:   []string{"ab", "cd", "ef\n", "gh\n"},
		},
		{
			prefix:   "p1 ",
			expected: "p1 ab\np1 cd\np1 ef\np1 gh\n",
			values:   []string{"ab\ncd\nef\ngh\n"},
		},
		{
			prefix:   "p1 ",
			expected: "p1 ab\np1 \np1 \np1 gh\n",
			values:   []string{"ab\n\n\ngh\n"},
		},
		{
			prefix:   "p1 ",
			expected: "",
			values:   []string{""},
		},
		{
			prefix:   "p1 ",
			expected: "p1 \n",
			values:   []string{"\n"},
		},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer

			pw := util.PrefixedWriter(&b, tc.prefix)
			for _, input := range tc.values {
				written, err := pw.Write([]byte(input))
				require.NoError(t, err)
				assert.Len(t, input, written)
			}

			assert.Equal(t, tc.expected, b.String())
		})
	}
}

type FailingWriter struct{}

func (fw *FailingWriter) Write(b []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestPrefixWriterFail(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		prefix   string
		expected string
		values   []string
	}{
		{
			prefix:   "p1 ",
			expected: "p1 ab",
			values:   []string{"a", "b"},
		},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			pw := util.PrefixedWriter(&FailingWriter{}, tc.prefix)
			for _, input := range tc.values {
				written, err := pw.Write([]byte(input))
				require.Error(t, err)
				assert.Empty(t, written)
			}
		})
	}
}
