package config //nolint:testpackage // needs access to unexported function under test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRewriteINISopsMetadataNewlinesCRLF(t *testing.T) {
	t.Parallel()

	input := "[sops]\r\n" +
		"  key = value\\nwith\\nlines\r\n" +
		"normal = keep-unchanged\r\n" +
		"[other]\r\n" +
		"line = untouched\\ntext\r\n"

	output := rewriteINISopsMetadataNewlines([]byte(input))

	assert.Equal(t, "[sops]\n  key = \"\"\"value\nwith\nlines\"\"\"\nnormal = keep-unchanged\n[other]\nline = untouched\\ntext\n", string(output))
}

func TestRewriteINISopsMetadataNewlinesNoSopsChanges(t *testing.T) {
	t.Parallel()

	input := "[other]\r\n" +
		"line = untouched\\ntext\r\n"

	output := rewriteINISopsMetadataNewlines([]byte(input))

	assert.Equal(t, "[other]\nline = untouched\\ntext\n", string(output))
}

func TestRewriteINISopsMetadataNewlinesNoEscapedNewlinesKeepsData(t *testing.T) {
	t.Parallel()

	input := "[sops]\r\nkey = plain\r\n"

	output := rewriteINISopsMetadataNewlines([]byte(input))

	assert.Equal(t, input, string(output))
}
