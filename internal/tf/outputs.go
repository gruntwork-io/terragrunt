package tf

import (
	"slices"

	"github.com/hashicorp/hcl/v2"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

var outputBlockSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "output",
			LabelNames: []string{"name"},
		},
	},
}

// ModuleOutputs returns the name of every output the OpenTofu/Terraform files of the module at modulePath declare,
// sorted and without repeats.
func ModuleOutputs(fsys vfs.FS, modulePath string) ([]string, error) {
	body, err := moduleBody(fsys, modulePath)
	if err != nil {
		return nil, err
	}

	content, _, diags := body.PartialContent(outputBlockSchema)
	if diags.HasErrors() {
		return nil, diags
	}

	names := make([]string, 0, len(content.Blocks))

	for _, block := range content.Blocks {
		names = append(names, block.Labels[0])
	}

	slices.Sort(names)

	return slices.Compact(names), nil
}
