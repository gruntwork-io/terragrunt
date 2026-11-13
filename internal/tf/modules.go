package tf

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

var moduleBlockSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "module",
			LabelNames: []string{"name"},
		},
	},
}

// HasModuleCalls reports whether the root module at modulePath declares a module block.
//
// Only the configuration files directly under modulePath are read, as those are the ones
// OpenTofu/Terraform load: a module block in a subdirectory, such as an examples/ tree that calls
// the module, does not count. The files are parsed, so a block in a comment or a string does not
// count either. A file that fails to parse counts as a module call, so the caller runs init and
// OpenTofu/Terraform report the syntax error.
func HasModuleCalls(fsys vfs.FS, modulePath string) (bool, error) {
	files, err := util.ListTfFiles(fsys, modulePath)
	if err != nil {
		return false, err
	}

	parser := hclparse.NewParser()

	for _, path := range files {
		src, err := vfs.ReadFile(fsys, path)
		if err != nil {
			return false, err
		}

		parseFunc := parser.ParseHCL
		if strings.HasSuffix(path, ".json") {
			parseFunc = parser.ParseJSON
		}

		file, diags := parseFunc(src, path)
		if diags.HasErrors() {
			return true, nil
		}

		content, _, diags := file.Body.PartialContent(moduleBlockSchema)
		if diags.HasErrors() || len(content.Blocks) > 0 {
			return true, nil
		}
	}

	return false, nil
}
