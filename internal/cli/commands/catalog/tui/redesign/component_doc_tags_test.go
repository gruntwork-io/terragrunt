package redesign_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/stretchr/testify/assert"
)

func TestComponentDoc_Tags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name: "inline array",
			content: `<!-- Frontmatter
name: VPC App
tags: [networking, aws, module]
-->
# VPC App
`,
			want: []string{"networking", "aws", "module"},
		},
		{
			name: "quoted inline array",
			content: `<!-- Frontmatter
name: VPC App
tags: ["networking", "aws"]
-->
# VPC App
`,
			want: []string{"networking", "aws"},
		},
		{
			name: "dash list",
			content: `<!-- Frontmatter
name: VPC App
tags:
  - networking
  - aws
  - module
-->
# VPC App
`,
			want: []string{"networking", "aws", "module"},
		},
		{
			name: "no tags key",
			content: `<!-- Frontmatter
name: VPC App
-->
# VPC App
`,
			want: nil,
		},
		{
			name: "no frontmatter",
			content: `# VPC App
Body only.
`,
			want: nil,
		},
		{
			name: "malformed yaml falls back to nil",
			content: `<!-- Frontmatter
name: VPC App
tags: [unterminated
-->
# VPC App
`,
			want: nil,
		},
		{
			name: "dash-separated frontmatter",
			content: `---
name: VPC App
tags: [networking, aws]
---
# VPC App
`,
			want: []string{"networking", "aws"},
		},
		{
			name: "dash-separated dash list",
			content: `---
name: VPC App
tags:
  - networking
  - aws
---
# VPC App
`,
			want: []string{"networking", "aws"},
		},
		{
			name: "empty entries are dropped",
			content: `<!-- Frontmatter
tags: ["", "aws", "  "]
-->
# VPC App
`,
			want: []string{"aws"},
		},
		{
			name: "boolean-shaped scalars preserve source text",
			content: `<!-- Frontmatter
tags: [no, yes, true, false]
-->
# VPC App
`,
			want: []string{"no", "yes", "true", "false"},
		},
		{
			name: "numeric scalars preserve source text",
			content: `<!-- Frontmatter
tags: [123, 000123, 0x10, 1.0]
-->
# VPC App
`,
			want: []string{"123", "000123", "0x10", "1.0"},
		},
		{
			name: "null entries are dropped",
			content: `<!-- Frontmatter
tags: [foo, ~, null, bar]
-->
# VPC App
`,
			want: []string{"foo", "bar"},
		},
		{
			name: "single scalar tag",
			content: `<!-- Frontmatter
tags: networking
-->
# VPC App
`,
			want: []string{"networking"},
		},
		{
			name: "non-scalar items are skipped",
			content: `<!-- Frontmatter
tags:
  - foo
  - {nested: value}
  - bar
-->
# VPC App
`,
			want: []string{"foo", "bar"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := redesign.NewComponentDoc(tc.content, ".md")
			assert.Equal(t, tc.want, doc.Tags())
		})
	}
}

// TestComponentDoc_TagsPreservesNameAndDescription guards against the YAML
// parser change regressing the existing name/description extraction.
func TestComponentDoc_TagsPreservesNameAndDescription(t *testing.T) {
	t.Parallel()

	content := `<!-- Frontmatter
name: VPC App
description: A VPC for application workloads.
tags: [networking, aws]
-->
# VPC App
`

	doc := redesign.NewComponentDoc(content, ".md")
	assert.Equal(t, "VPC App", doc.Title())
	assert.Equal(t, "A VPC for application workloads.", doc.Description(0))
	assert.Equal(t, []string{"networking", "aws"}, doc.Tags())
}
