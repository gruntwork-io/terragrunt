package config_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindInputsReads(t *testing.T) {
	t.Parallel()

	type read struct {
		function string
		expr     string
		line     int
		marked   bool
	}

	testCases := []struct {
		name     string
		filename string
		content  string
		want     []read
	}{
		{
			name:     "no inputs attribute",
			filename: "terragrunt.hcl",
			content: `
locals {
  config = yamldecode(file("config.yml"))
}
`,
		},
		{
			name:     "inputs without reads",
			filename: "terragrunt.hcl",
			content: `
locals {
  config = yamldecode(file("config.yml"))
}

inputs = {
  message = local.config.message
  region  = get_env("AWS_REGION")
}
`,
		},
		{
			name:     "reads nested in other calls",
			filename: "terragrunt.hcl",
			content: `
inputs = merge(local.defaults, {
  message = yamldecode(file("config.yml")).message
  vars    = read_tfvars_file("shared.tfvars")
  marked  = mark_as_read("read-by-tofu.txt")
})
`,
			want: []read{
				{function: "file", expr: `file("config.yml")`, line: 3},
				{function: "read_tfvars_file", expr: `read_tfvars_file("shared.tfvars")`, line: 4},
				{function: "mark_as_read", expr: `mark_as_read("read-by-tofu.txt")`, line: 5, marked: true},
			},
		},
		{
			name:     "templatefile points at its path",
			filename: "terragrunt.hcl",
			content: `
inputs = {
  policy = templatefile("policy.json.tftpl", { role_arn = dependency.iam.outputs.role_arn })
}
`,
			want: []read{{function: "templatefile", expr: `"policy.json.tftpl"`, line: 3}},
		},
		{
			name:     "file named by a dependency output",
			filename: "terragrunt.hcl",
			content: `
inputs = {
  config   = yamldecode(file("${dependency.vpc.outputs.config_dir}/config.yml"))
  policy   = templatefile(dependency.iam.outputs.policy_template, { role_arn = local.role_arn })
  settings = read_terragrunt_config(find_in_parent_folders(dependency.vpc.outputs.settings_file))
  marked   = mark_as_read("read-by-tofu.txt")
}
`,
			want: []read{
				{function: "mark_as_read", expr: `mark_as_read("read-by-tofu.txt")`, line: 6, marked: true},
			},
		},
		{
			name:     "path marked as read",
			filename: "terragrunt.hcl",
			content: `
inputs = {
  policy  = templatefile(mark_as_read("policy.json.tftpl"), { role_arn = dependency.iam.outputs.role_arn })
  message = file(mark_as_read("message.txt"))
  listed  = [for name in local.names : mark_as_read("${name}.txt")]
}
`,
			want: []read{
				{function: "templatefile", expr: `mark_as_read("policy.json.tftpl")`, line: 3, marked: true},
				{function: "mark_as_read", expr: `mark_as_read("policy.json.tftpl")`, line: 3, marked: true},
				{function: "file", expr: `file(mark_as_read("message.txt"))`, line: 4, marked: true},
				{function: "mark_as_read", expr: `mark_as_read("message.txt")`, line: 4, marked: true},
				{function: "mark_as_read", expr: `mark_as_read("${name}.txt")`, line: 5, marked: true},
			},
		},
		{
			name:     "path marked with the variable of a loop over a dependency output",
			filename: "terragrunt.hcl",
			content: `
inputs = {
  rendered = [for name in dependency.iam.outputs.names : templatefile(mark_as_read("${name}.tftpl"), {})]
  listed   = [for name in local.names : mark_as_read("${name}.txt")]
  shared   = [for arn in dependency.iam.outputs.arns : templatefile(mark_as_read("shared.tftpl"), { arn = arn })]
}
`,
			want: []read{
				{function: "templatefile", expr: `mark_as_read("${name}.tftpl")`, line: 3},
				{function: "mark_as_read", expr: `mark_as_read("${name}.tftpl")`, line: 3},
				{function: "mark_as_read", expr: `mark_as_read("${name}.txt")`, line: 4, marked: true},
				{function: "templatefile", expr: `mark_as_read("shared.tftpl")`, line: 5, marked: true},
				{function: "mark_as_read", expr: `mark_as_read("shared.tftpl")`, line: 5, marked: true},
			},
		},
		{
			name:     "templatefile without arguments",
			filename: "terragrunt.hcl",
			content: `
inputs = {
  policy = templatefile()
}
`,
			want: []read{{function: "templatefile", expr: `templatefile()`, line: 3}},
		},
		{
			name:     "json config",
			filename: "terragrunt.hcl.json",
			content:  `{"inputs": {"message": "${file(\"config.yml\")}"}}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := hclparse.NewParser().ParseFromString(tc.content, tc.filename)
			require.NoError(t, err)

			reads, diags := config.FindInputsReads(file.Body)
			require.False(t, diags.HasErrors(), diags.Error())

			got := make([]read, 0, len(reads))
			for _, r := range reads {
				assert.Equal(t, tc.filename, r.Range.Filename)

				got = append(got, read{
					function: r.Function,
					expr:     string(r.Range.SliceBytes(file.Bytes)),
					line:     r.Range.Start.Line,
					marked:   r.MarkedAsRead,
				})
			}

			if len(tc.want) == 0 {
				assert.Empty(t, got)

				return
			}

			assert.Equal(t, tc.want, got)
		})
	}
}
