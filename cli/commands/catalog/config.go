package catalog

import (
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/sirupsen/logrus"
)

const (
	// A consistent error message for multiple catalog block in terragrunt config (which is currently not supported)
	multipleCatalogBlockDetail = "Terragrunt currently does not support multiple catalog blocks in a single config. Consolidate to a single catalog block."
)

type Config struct {
	URLs []string `hcl:"urls,attr"`
}

func NewConfig() *Config {
	return &Config{}
}

func (cfg *Config) Load(configPath string) error {
	log.Debugf("Reading Terragrunt cfg file at %s", configPath)

	configString, err := util.ReadFileAsString(configPath)
	if err != nil {
		return err
	}

	// Parse the HCL string into an AST body that can be decoded multiple times later without having to re-parse
	parser := hclparse.NewParser()
	hclFile, err := config.ParseHCL(parser, configString, configPath)
	if err != nil {
		return err
	}

	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(
				config.PanicWhileParsingConfig{
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
func (cfg *Config) decodeCatalogBlock(hclFile *hcl.File) hcl.Diagnostics {
	block, diags := getCatalogBlock(hclFile)
	if diags.HasErrors() {
		return diags
	} else if block == nil {
		// No catalog block referenced in the file
		log.Debugf("Did not find any catalog block: skipping evaluation.")
		return nil
	}

	if diags := gohcl.DecodeBody(block.Body, nil, cfg); diags.HasErrors() {
		return diags
	}

	return diags
}

func (cfg *Config) normalize(cofnigPath string) {
	cfg.URLs = util.RemoveDuplicatesFromList(cfg.URLs)
	configDir := filepath.Dir(cofnigPath)

	// transform relative paths to absolute ones
	for i, url := range cfg.URLs {
		url := filepath.Join(configDir, url)

		if files.FileExists(url) {
			cfg.URLs[i] = url
		}
	}
}

// getCatalogBlock takes a parsed HCL file and extracts a reference to the `catalog` block, if there is one defined.
func getCatalogBlock(hclFile *hcl.File) (*hcl.Block, hcl.Diagnostics) {
	localsSchema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			hcl.BlockHeaderSchema{Type: "catalog"},
		},
	}
	// We use PartialContent here, because we are only interested in parsing out the catalog block.
	parsedLocals, _, diags := hclFile.Body.PartialContent(localsSchema)
	extractedLocalsBlocks := []*hcl.Block{}
	for _, block := range parsedLocals.Blocks {
		if block.Type == "catalog" {
			extractedLocalsBlocks = append(extractedLocalsBlocks, block)
		}
	}
	// We currently only support parsing a single catalog block
	switch {
	case len(extractedLocalsBlocks) == 1:
		return extractedLocalsBlocks[0], diags
	case len(extractedLocalsBlocks) > 1:
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Multiple catalog block",
			Detail:   multipleCatalogBlockDetail,
		})
		return nil, diags
	default:
		// No catalog block parsed
		return nil, diags
	}
}

func findFileInParentDirs(configPath string) string {
	if files.IsDir(configPath) {
		return ""
	}

	if files.FileExists(configPath) {
		return configPath
	}

	currentDir, filename := filepath.Split(configPath)
	parentDir := filepath.Dir(filepath.Dir(currentDir))

	// if the current directory is the root path, stop searching
	if parentDir == currentDir {
		return ""
	}

	configPath = filepath.Join(parentDir, filename)
	return findFileInParentDirs(configPath)
}
