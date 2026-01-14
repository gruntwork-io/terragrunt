// Package render provides the command to render the final terragrunt config in various formats.
package render

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/runner"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/prepare"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

func Run(ctx context.Context, l log.Logger, opts *Options) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	if opts.RunAll {
		return runAll(ctx, l, opts)
	}

	prepared, err := prepare.PrepareConfig(ctx, l, opts.TerragruntOptions)
	if err != nil {
		return err
	}

	return runRender(ctx, l, opts, prepared.Cfg)
}

func runAll(ctx context.Context, l log.Logger, opts *Options) error {
	d := discovery.NewDiscovery(opts.WorkingDir)

	components, err := d.Discover(ctx, l, opts.TerragruntOptions)
	if err != nil {
		return err
	}

	units := components.Filter(component.UnitKind).Sort()

	var errs []error

	for _, unit := range units {
		unitOpts := opts.Clone()
		unitOpts.WorkingDir = unit.Path()

		configFilename := config.DefaultTerragruntConfigPath
		if len(opts.TerragruntConfigPath) > 0 {
			configFilename = filepath.Base(opts.TerragruntConfigPath)
		}

		unitOpts.TerragruntConfigPath = filepath.Join(unit.Path(), configFilename)

		prepared, err := prepare.PrepareConfig(ctx, l, unitOpts.TerragruntOptions)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if err := runRender(ctx, l, unitOpts, prepared.Cfg); err != nil {
			if opts.FailFast {
				return err
			}

			errs = append(
				errs,
				fmt.Errorf(
					"render of unit %s failed: %w",
					unit.Path(),
					err,
				),
			)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func runRender(ctx context.Context, l log.Logger, opts *Options, cfg *config.TerragruntConfig) error {
	if cfg == nil {
		return errors.New("terragrunt was not able to render the config because it received no config. This is almost certainly a bug in Terragrunt. Please open an issue on github.com/gruntwork-io/terragrunt with this message and the contents of your terragrunt.hcl")
	}

	switch opts.Format {
	case FormatJSON:
		return renderJSON(ctx, l, opts, cfg)
	case FormatHCL:
		return renderHCL(ctx, l, opts, cfg)
	default:
		return fmt.Errorf("unsupported render format: %s", opts.Format)
	}
}

func renderHCL(_ context.Context, l log.Logger, opts *Options, cfg *config.TerragruntConfig) error {
	if opts.Write {
		buf := new(bytes.Buffer)

		_, err := cfg.WriteTo(buf)
		if err != nil {
			return err
		}

		return writeRendered(l, opts, buf.Bytes())
	}

	l.Infof("Rendering config %s", opts.TerragruntConfigPath)

	_, err := cfg.WriteTo(opts.Writer)
	if err != nil {
		return err
	}

	return nil
}

func renderJSON(ctx context.Context, l log.Logger, opts *Options, cfg *config.TerragruntConfig) error {
	if !opts.DisableDependentModules {
		dependentModules := runner.FindWhereWorkingDirIsIncluded(ctx, l, opts.TerragruntOptions, cfg)

		dependentModulesPath := make([]*string, 0, len(dependentModules))
		for _, module := range dependentModules {
			path := module.Path()
			dependentModulesPath = append(dependentModulesPath, &path)
		}

		cfg.DependentModulesPath = dependentModulesPath
		cfg.SetFieldMetadata(config.MetadataDependentModules, map[string]any{config.FoundInFile: opts.TerragruntConfigPath})
	}

	var terragruntConfigCty cty.Value

	if opts.RenderMetadata {
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

	if opts.Write {
		return writeRendered(l, opts, jsonBytes)
	}

	l.Infof("Rendering config %s", opts.TerragruntConfigPath)

	_, err = opts.Writer.Write(jsonBytes)
	if err != nil {
		return errors.New(err)
	}

	return nil
}

func writeRendered(l log.Logger, opts *Options, data []byte) error {
	outPath := opts.OutputPath
	if !filepath.IsAbs(outPath) {
		terragruntConfigDir := filepath.Dir(opts.TerragruntConfigPath)
		outPath = filepath.Join(terragruntConfigDir, outPath)
	}

	if err := util.EnsureDirectory(filepath.Dir(outPath)); err != nil {
		return err
	}

	l.Debugf("Rendering config %s to %s", opts.TerragruntConfigPath, outPath)

	const ownerWriteGlobalReadPerms = 0644
	if err := os.WriteFile(outPath, data, ownerWriteGlobalReadPerms); err != nil {
		return errors.New(err)
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
		return nil, errors.New(err)
	}

	var ctyJSONOutput ctyhelper.CtyJSONOutput
	if err = json.Unmarshal(jsonBytesIntermediate, &ctyJSONOutput); err != nil {
		return nil, errors.New(err)
	}

	jsonBytes, err := json.Marshal(ctyJSONOutput.Value)
	if err != nil {
		return nil, errors.New(err)
	}

	jsonBytes = append(jsonBytes, '\n')

	return jsonBytes, nil
}
