package util_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestWrapWords(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		in    string
		want  string
		width int
	}{
		{
			name:  "empty input",
			in:    "",
			want:  "",
			width: 10,
		},
		{
			name:  "no wrap needed",
			in:    "short line",
			want:  "short line",
			width: 80,
		},
		{
			name:  "wraps at space boundary",
			in:    "one two three four five",
			want:  "one two\nthree four\nfive",
			width: 10,
		},
		{
			name:  "word longer than width goes on its own line",
			in:    "tiny supercalifragilisticexpialidocious tail",
			want:  "tiny\nsupercalifragilisticexpialidocious\ntail",
			width: 10,
		},
		{
			name:  "preserves embedded newlines as hard breaks",
			in:    "first paragraph here\n\nsecond paragraph too",
			want:  "first paragraph here\n\nsecond paragraph too",
			width: 80,
		},
		{
			name:  "wraps each paragraph independently",
			in:    "one two three four five\nsix seven eight nine ten",
			want:  "one two\nthree four\nfive\nsix seven\neight nine\nten",
			width: 10,
		},
		{
			name:  "collapses runs of internal whitespace",
			in:    "one    two\t\tthree",
			want:  "one two three",
			width: 80,
		},
		{
			name:  "counts runes not bytes for multibyte characters",
			in:    "αβ γδ εζ ηθ ικ",
			want:  "αβ γδ\nεζ ηθ\nικ",
			width: 5,
		},
		{
			name:  "zero width returns input unchanged",
			in:    "anything goes here",
			want:  "anything goes here",
			width: 0,
		},
		{
			name:  "negative width returns input unchanged",
			in:    "anything goes here",
			want:  "anything goes here",
			width: -5,
		},
		{
			name:  "width exactly fits one word per line",
			in:    "abc def ghi",
			want:  "abc\ndef\nghi",
			width: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := util.WrapWords(tc.in, tc.width)
			assert.Equal(t, tc.want, got)
		})
	}
}
