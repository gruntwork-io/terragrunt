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
		expected string
		values   []string
	}{
		{prefix: "p1 ", values: []string{"a", "b"}, expected: "p1 ab"},
		{prefix: "p2 ", values: []string{"a", "b"}, expected: "p2 ab"},

		{prefix: "", values: []string{"a", "b"}, expected: "ab"},

		{prefix: "p1 ", values: []string{"a", "b\n"}, expected: "p1 ab\n"},
		{prefix: "p1 ", values: []string{"a\n", "b"}, expected: "p1 a\np1 b"},
		{prefix: "p1 ", values: []string{"a\n", "b\n"}, expected: "p1 a\np1 b\n"},
		{prefix: "p1 ", values: []string{"a", "b", "c", "def"}, expected: "p1 abcdef"},
		{prefix: "p1 ", values: []string{"a", "b\n", "c", "def"}, expected: "p1 ab\np1 cdef"},
		{prefix: "p1 ", values: []string{"a", "b\nc", "def"}, expected: "p1 ab\np1 cdef"},
		{prefix: "p1 ", values: []string{"ab", "cd", "ef", "gh\n"}, expected: "p1 abcdefgh\n"},
		{prefix: "p1 ", values: []string{"ab", "cd\n", "ef", "gh\n"}, expected: "p1 abcd\np1 efgh\n"},
		{prefix: "p1 ", values: []string{"ab", "cd", "e\nf", "gh\n"}, expected: "p1 abcde\np1 fgh\n"},
		{prefix: "p1 ", values: []string{"ab", "cd", "ef\n", "gh\n"}, expected: "p1 abcdef\np1 gh\n"},
		{prefix: "p1 ", values: []string{"ab\ncd\nef\ngh\n"}, expected: "p1 ab\np1 cd\np1 ef\np1 gh\n"},
		{prefix: "p1 ", values: []string{"ab\n\n\ngh\n"}, expected: "p1 ab\np1 \np1 \np1 gh\n"},

		{prefix: "p1 ", values: []string{""}, expected: ""},
		{prefix: "p1 ", values: []string{"\n"}, expected: "p1 \n"},
		{prefix: "p1\n", values: []string{"\n"}, expected: "p1\n\n"},
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
		expected string
		values   []string
	}{
		{prefix: "p1 ", values: []string{"a", "b"}, expected: "p1 ab"},
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
