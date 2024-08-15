package config

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/zclconf/go-cty/cty"
)

const (
	rootConfigFmt = `
include "root" {
  path = find_in_parent_folders("%s")
}
`
	// matches a block and ignores commented out config, where the block name is passed as the first argument to fmt, e.g.
	// `fmt.Sprintf(hclBlockRegExprFmt, "include")` returns a regexp expression matching the `include` block:
	//
	// ```hcl
	// include {
	//
	// }
	// ```.
	hclBlockRegExprFmt = `(?is)(?:^|^((?:[^/]|/[^*])*)(?:/\*.*?\*/)?((?:[^/]|/[^*])*)\n)(%s[ {][^\}]+)`
)

var (
	includeBlockReg = regexp.MustCompile(fmt.Sprintf(hclBlockRegExprFmt, MetadataInclude))
	catalogBlockReg = regexp.MustCompile(fmt.Sprintf(hclBlockRegExprFmt, MetadataCatalog))
)

// CatalogConfig represents the configuration for the `catalog` block.
type CatalogConfig struct {
	URLs []string `cty:"urls" hcl:"urls,attr"`
}

// String returns a string representation of the CatalogConfig.
func (cfg *CatalogConfig) String() string {
	return fmt.Sprintf("Catalog{URLs = %v}", cfg.URLs)
}

// normalize transforms relative paths to absolute ones.
func (cfg *CatalogConfig) normalize(configPath string) {
	configDir := filepath.Dir(configPath)

	// transform relative paths to absolute ones
	for i, url := range cfg.URLs {
		url := filepath.Join(configDir, url)

		if files.FileExists(url) {
			cfg.URLs[i] = url
		}
	}
}

// ReadCatalogConfig reads the `catalog` block from the `terragrunt.hcl` file.
//
// We want users to be able to browse to any folder in an `infra-live` repo,
// and run `terragrunt catalog` extra arguments.
//
// ReadCatalogConfig looks for the "nearest" `terragrunt.hcl` in parent directories
// if the given `opts.TerragruntConfigPath` does not exist.
//
// Since our normal parsing `ParseConfig` does not always work, as some `terragrunt.hcl` files are meant to be used
// from an `include` and/or they might use `find_in_parent_folders` only work from certain child folders,
// it parses this file to see if the config contains `include{...find_in_parent_folders()...}` block to determine
// if it is the root configuration.
//
// If it finds that the `terragrunt.hcl` already has `include`,
// then it reads that configuration as is, otherwise generate a stub child `terragrunt.hcl`
// in memory with an `include` to pull in the one we found.
//
// Unlike the "ReadTerragruntConfig" func, it ignores any configuration errors not related to the "catalog" block.
func ReadCatalogConfig(parentCtx context.Context, opts *options.TerragruntOptions) (*CatalogConfig, error) {
	configPath, configString, err := findCatalogConfig(parentCtx, opts)
	if err != nil || configPath == "" {
		return nil, err
	}

	opts.TerragruntConfigPath = configPath

	ctx := NewParsingContext(parentCtx, opts)
	ctx.ParserOptions = append(ctx.ParserOptions, hclparse.WithHaltOnErrorOnlyForBlocks([]string{MetadataCatalog}))
	ctx.ConvertToTerragruntConfigFunc = convertToTerragruntCatalogConfig

	// TODO: Resolve lint error
	config, err := ParseConfigString(ctx, configPath, configString, nil) //nolint:contextcheck
	if err != nil {
		return nil, err
	}

	return config.Catalog, nil
}

func findCatalogConfig(ctx context.Context, opts *options.TerragruntOptions) (string, string, error) {
	var (
		configPath        = opts.TerragruntConfigPath
		configName        = filepath.Base(configPath)
		catalogConfigPath string
	)

	for {
		opts = &options.TerragruntOptions{
			TerragruntConfigPath: filepath.Join(filepath.Dir(configPath), util.UniqueId(), configName),
			MaxFoldersToCheck:    opts.MaxFoldersToCheck,
		}

		// This allows to stop the process by pressing Ctrl-C, in case the loop is endless,
		// it can happen if the functions of the `filepath` package do not work correctly under a certain operating system.
		select {
		case <-ctx.Done():
			return "", "", nil
		default:
		}

		newConfigPath, err := FindInParentFolders(NewParsingContext(ctx, opts), []string{configName})
		if err != nil {
			var parentFileNotFoundError ParentFileNotFoundError
			if ok := errors.As(err, &parentFileNotFoundError); ok {
				break
			}

			return "", "", err
		}

		configString, err := util.ReadFileAsString(newConfigPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to read file %s: %w", newConfigPath, err)
		}

		// if the config contains `include` block (root config), read the config as is.
		if includeBlockReg.MatchString(configString) {
			return newConfigPath, configString, nil
		}

		// if the config contains a `catalog` block, save the path in case the root config is not found
		if catalogBlockReg.MatchString(configString) {
			catalogConfigPath = newConfigPath
		}

		configPath = filepath.Dir(newConfigPath)
	}

	// if the config with the `catalog` block is found, create the root config with `include{ find_in_parent_folders() }`
	// and the path one directory deeper in order for `find_in_parent_folders` can find the catalog configuration.
	if catalogConfigPath != "" {
		configString := fmt.Sprintf(rootConfigFmt, configName)
		configPath = filepath.Join(filepath.Dir(catalogConfigPath), util.UniqueId(), configName)

		return configPath, configString, nil
	}

	return "", "", nil
}

func convertToTerragruntCatalogConfig(ctx *ParsingContext, configPath string, terragruntConfigFromFile *terragruntConfigFile) (*TerragruntConfig, error) { //nolint:lll
	var (
		terragruntConfig = &TerragruntConfig{}
		defaultMetadata  = map[string]interface{}{FoundInFile: configPath}
	)

	if terragruntConfigFromFile.Catalog != nil {
		terragruntConfig.Catalog = terragruntConfigFromFile.Catalog
		terragruntConfig.Catalog.normalize(configPath)
		terragruntConfig.SetFieldMetadata(MetadataCatalog, defaultMetadata)
	}

	if terragruntConfigFromFile.Engine != nil {
		terragruntConfig.Engine = terragruntConfigFromFile.Engine
		terragruntConfig.SetFieldMetadata(MetadataEngine, defaultMetadata)
	}

	if ctx.Locals != nil && *ctx.Locals != cty.NilVal {
		// We should ignore any errors from `parseCtyValueToMap` as some `locals` values might have been
		// incorrectly evaluated, which results in an `json.Unmarshal` error.
		//
		// For example if the locals block looks like `{"var1":, "var2":"value2"}`,
		// `parseCtyValueToMap` returns the map with "var2" value and a syntax error.
		//
		// Since we consciously understand that not all variables can be evaluated correctly due to
		// the fact that parsing may not start from the real root file, we can safely ignore this error.
		localsParsed, _ := ParseCtyValueToMap(*ctx.Locals)
		terragruntConfig.Locals = localsParsed
		terragruntConfig.SetFieldMetadataMap(MetadataLocals, localsParsed, defaultMetadata)
	}

	return terragruntConfig, nil
}
