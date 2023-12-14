package config

import (
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/sirupsen/logrus"
)

type Catalog struct {
	URLs []string `hcl:"urls,attr"`
}

func NewCatalog() *Catalog {
	return &Catalog{}
}

func (cfg *Catalog) ParseConfigFile(configPath string) error {
	log.Debugf("Reading Terragrunt cfg file at %s", configPath)

	configString, err := util.ReadFileAsString(configPath)
	if err != nil {
		return err
	}

	// Parse the HCL string into an AST body that can be decoded multiple times later without having to re-parse
	parser := hclparse.NewParser()
	hclFile, err := parseHcl(parser, configString, configPath)
	if err != nil {
		return err
	}

	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(
				PanicWhileParsingConfig{
					RecoveredValue: recovered,
					ConfigFile:     configPath,
				},
			)
		}
	}()

	diagsWriter := util.GetDiagnosticsWriter(logrus.NewEntry(log.Logger()), parser)

	if diags := cfg.decodeCatalogBlock(hclFile); diags != nil {
		log.Errorf("Encountered error while decoding catalog block into name expression pairs.")
		err := diagsWriter.WriteDiagnostics(diags)
		if err != nil {
			return errors.WithStackTrace(err)
		}
		return errors.WithStackTrace(diags)
	}

	cfg.normalize(configPath)

	return nil
}

// decodeCatalogBlock loads the block into name expression pairs to assist with evaluation of the catalog prior to
// evaluating the whole config. Note that this is exactly the same as
// terraform/configs/named_values.go:decodeCatalogBlock
func (cfg *Catalog) decodeCatalogBlock(hclFile *hcl.File) hcl.Diagnostics {
	block, diags := getBlock(hclFile, "catalog", false)
	if diags.HasErrors() {
		return diags
	} else if len(block) == 0 {
		// No catalog block referenced in the file
		log.Debugf("Did not find any catalog block: skipping evaluation.")
		return nil
	}

	if diags := gohcl.DecodeBody(block[0].Body, nil, cfg); diags.HasErrors() {
		return diags
	}

	return diags
}

func (cfg *Catalog) normalize(cofnigPath string) {
	configDir := filepath.Dir(cofnigPath)

	// transform relative paths to absolute ones
	for i, url := range cfg.URLs {
		url := filepath.Join(configDir, url)

		if files.FileExists(url) {
			cfg.URLs[i] = url
		}
	}
}
