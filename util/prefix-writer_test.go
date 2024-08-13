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

	tc := []struct {
		prefix   string
		values   []string
		expected string
	}{
		{"p1 ", []string{"a", "b"}, "p1 ab"},
		{"p2 ", []string{"a", "b"}, "p2 ab"},

		{"", []string{"a", "b"}, "ab"},

		{"p1 ", []string{"a", "b\n"}, "p1 ab\n"},
		{"p1 ", []string{"a\n", "b"}, "p1 a\np1 b"},
		{"p1 ", []string{"a\n", "b\n"}, "p1 a\np1 b\n"},
		{"p1 ", []string{"a", "b", "c", "def"}, "p1 abcdef"},
		{"p1 ", []string{"a", "b\n", "c", "def"}, "p1 ab\np1 cdef"},
		{"p1 ", []string{"a", "b\nc", "def"}, "p1 ab\np1 cdef"},
		{"p1 ", []string{"ab", "cd", "ef", "gh\n"}, "p1 abcdefgh\n"},
		{"p1 ", []string{"ab", "cd\n", "ef", "gh\n"}, "p1 abcd\np1 efgh\n"},
		{"p1 ", []string{"ab", "cd", "e\nf", "gh\n"}, "p1 abcde\np1 fgh\n"},
		{"p1 ", []string{"ab", "cd", "ef\n", "gh\n"}, "p1 abcdef\np1 gh\n"},
		{"p1 ", []string{"ab\ncd\nef\ngh\n"}, "p1 ab\np1 cd\np1 ef\np1 gh\n"},
		{"p1 ", []string{"ab\n\n\ngh\n"}, "p1 ab\np1 \np1 \np1 gh\n"},

		{"p1 ", []string{""}, ""},
		{"p1 ", []string{"\n"}, "p1 \n"},
		{"p1\n", []string{"\n"}, "p1\n\n"},
	}

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer
			pw := util.PrefixedWriter(&b, tt.prefix)
			for _, input := range tt.values {
				written, err := pw.Write([]byte(input))
				require.NoError(t, err)
				assert.Len(t, input, written)
			}
			assert.Equal(t, tt.expected, b.String())
		})
	}
}

type FailingWriter struct{}

func (fw *FailingWriter) Write(b []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestPrefixWriterFail(t *testing.T) {
	t.Parallel()

	tc := []struct {
		prefix   string
		values   []string
		expected string
	}{
		{"p1 ", []string{"a", "b"}, "p1 ab"},
	}

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			pw := util.PrefixedWriter(&FailingWriter{}, tt.prefix)
			for _, input := range tt.values {
				written, err := pw.Write([]byte(input))
				require.Error(t, err)
				assert.Empty(t, written)
			}
		})
	}
}
