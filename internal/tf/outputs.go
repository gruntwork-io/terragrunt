package tf

import (
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/util"
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
// sorted and without repeats. The files are the ones impl reads, as [EngineFiles] selects them.
func ModuleOutputs(fsys vfs.FS, modulePath string, impl tfimpl.Type) ([]string, error) {
	files, err := util.ListTfFiles(fsys, modulePath)
	if err != nil {
		return nil, err
	}

	body, err := parseModuleFiles(fsys, EngineFiles(files, impl))
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

// EngineFiles returns the files of paths that impl reads. OpenTofu leaves out a .tf or .tf.json file beside the
// .tofu or .tofu.json file of the same name, Terraform leaves out every .tofu and .tofu.json file, and an unknown
// implementation reads them all.
func EngineFiles(paths []string, impl tfimpl.Type) []string {
	if impl == tfimpl.Terraform {
		return slices.DeleteFunc(slices.Clone(paths), func(path string) bool {
			return strings.HasSuffix(path, ".tofu") || strings.HasSuffix(path, ".tofu.json")
		})
	}

	if impl != tfimpl.OpenTofu {
		return paths
	}

	return slices.DeleteFunc(slices.Clone(paths), func(path string) bool {
		twin, ok := tofuTwin(path)

		return ok && slices.Contains(paths, twin)
	})
}

// tofuTwin returns the .tofu or .tofu.json file that hides path, a .tf or .tf.json file, from OpenTofu.
func tofuTwin(path string) (string, bool) {
	for _, ext := range []string{".tf.json", ".tf"} {
		base, ok := strings.CutSuffix(path, ext)
		if !ok {
			continue
		}

		return base + strings.Replace(ext, ".tf", ".tofu", 1), true
	}

	return "", false
}
