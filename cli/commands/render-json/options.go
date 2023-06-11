package renderjson

import "github.com/gruntwork-io/terragrunt/options"

const (
	defaultJSONOutName = "terragrunt_rendered.json"
)

type Options struct {
	*options.TerragruntOptions

	// The file path that terragrunt should use when rendering the terragrunt.hcl config as json.
	JSONOut string

	// Include fields metadata in render-json
	RenderJsonWithMetadata bool
}

func NewOptions(global *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: global,
		JSONOut:           defaultJSONOutName,
	}
}
