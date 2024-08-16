// `render-json` command takes the parsed TerragruntConfig struct and renders it out as JSON
// so that it can be processed by other tools.
// To make it easier to maintain, this uses the cty representation as an intermediary.

// NOTE: An unspecified advantage of using the cty representation is that the final block outputs would be a map
// representation, which is easier to work with than the list representation that will be returned by a naive go-struct
// to json conversion.

package renderjson

import (
	"context"
	"encoding/json"
	goErrors "errors"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/configstack"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// Run the `render-json` command.
func Run(ctx context.Context, opts *options.TerragruntOptions) error {
	target := terraform.NewTarget(terraform.TargetPointParseConfig, runRenderJSON)

	err := terraform.RunWithTarget(ctx, opts, target)
	if err != nil {
		return err
	}

	return nil
}

var (
	// ErrNoConfig is returned when the TerragruntConfig is nil.
	ErrNoConfig = goErrors.New("terragrunt was not able to render the config as json because it received no config. This is almost certainly a bug in Terragrunt. Please open an issue on github.com/gruntwork-io/terragrunt with this message and the contents of your terragrunt.hcl") //nolint:lll
)

func runRenderJSON(ctx context.Context, opts *options.TerragruntOptions, cfg *config.TerragruntConfig) error {
	if cfg == nil {
		return ErrNoConfig
	}

	if !opts.JSONDisableDependentModules {
		dependentModules := configstack.FindWhereWorkingDirIsIncluded(ctx, opts, cfg)

		var dependentModulesPath []*string

		for _, module := range dependentModules {
			dependentModulesPath = append(dependentModulesPath, &module.Path)
		}

		cfg.DependentModulesPath = dependentModulesPath
		cfg.SetFieldMetadata(
			config.MetadataDependentModules,
			map[string]interface{}{
				config.FoundInFile: opts.TerragruntConfigPath,
			},
		)
	}

	var terragruntConfigCty cty.Value

	if opts.RenderJSONWithMetadata {
		cty, err := config.TerragruntConfigAsCtyWithMetadata(cfg)
		if err != nil {
			return err
		}

		terragruntConfigCty = cty
	} else {
		cty, err := config.TerragruntConfigAsCty(cfg)
		if err != nil {
			return err
		}

		terragruntConfigCty = cty
	}

	jsonBytes, err := marshalCtyValueJSONWithoutType(terragruntConfigCty)
	if err != nil {
		return err
	}

	jsonOutPath := opts.JSONOut
	if !filepath.IsAbs(jsonOutPath) {
		terragruntConfigDir := filepath.Dir(opts.TerragruntConfigPath)
		jsonOutPath = filepath.Join(terragruntConfigDir, jsonOutPath)
	}

	if err := util.EnsureDirectory(filepath.Dir(jsonOutPath)); err != nil {
		return err
	}

	opts.Logger.Debugf("Rendering config %s to JSON %s", opts.TerragruntConfigPath, jsonOutPath)

	const ownerWriteGlobalReadPerms = 0644

	if err := os.WriteFile(jsonOutPath, jsonBytes, ownerWriteGlobalReadPerms); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

// marshalCtyValueJSONWithoutType marshals the given cty.Value object into a JSON object that does not have the type.
// Using ctyjson directly would render a json object with two attributes, "value" and "type", and this function returns
// just the "value".
// NOTE: We have to do two marshalling passes so that we can extract just the value.
func marshalCtyValueJSONWithoutType(ctyVal cty.Value) ([]byte, error) {
	jsonBytesIntermediate, err := ctyjson.Marshal(ctyVal, cty.DynamicPseudoType)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	var ctyJSONOutput config.CtyJSONOutput

	if err := json.Unmarshal(jsonBytesIntermediate, &ctyJSONOutput); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	jsonBytes, err := json.Marshal(ctyJSONOutput.Value)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return jsonBytes, nil
}
