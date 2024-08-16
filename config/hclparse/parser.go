// The package wraps `hclparse.Parser` to be able to handle diagnostic errors from one place, see `handleDiagnostics(diags hcl.Diagnostics) error` func.
// This allows us to halt the process only when certain errors occur, such as skipping all errors not related to the `catalog` block.

package hclparse

import (
	"os"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

type Parser struct {
	*hclparse.Parser
	diagsWriterFunc       func(hcl.Diagnostics) error
	handleDiagnosticsFunc func(*File, hcl.Diagnostics) (hcl.Diagnostics, error)
	fileUpdateHandlerFunc func(*File) error
}

func NewParser() *Parser {
	return &Parser{
		Parser: hclparse.NewParser(),
	}
}

func (parser *Parser) WithOptions(opts ...Option) *Parser {
	for _, opt := range opts {
		parser = opt(parser)
	}
	return parser
}

func (parser *Parser) ParseFromFile(configPath string) (*File, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		log.Warnf("Error reading file %s: %v", configPath, err)
		return nil, errors.WithStackTrace(err)
	}

	return parser.ParseFromBytes(content, configPath)
}

// ParseFromString uses the HCL2 parser to parse the given string into an HCL file body.
func (parser *Parser) ParseFromString(content, configPath string) (file *File, err error) {
	return parser.ParseFromBytes([]byte(content), configPath)
}

func (parser *Parser) ParseFromBytes(content []byte, configPath string) (file *File, err error) {
	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(PanicWhileParsingConfigError{RecoveredValue: recovered, ConfigFile: configPath})
		}
	}()

	var (
		diags   hcl.Diagnostics
		hclFile *hcl.File
	)

	switch filepath.Ext(configPath) {
	case ".json":
		hclFile, diags = parser.ParseJSON(content, configPath)
	default:
		hclFile, diags = parser.ParseHCL(content, configPath)
	}

	file = &File{
		Parser:     parser,
		File:       hclFile,
		ConfigPath: configPath,
	}

	if err := parser.handleDiagnostics(file, diags); err != nil {
		log.Warnf("Failed to parse HCL in file %s: %v", configPath, diags)
		return nil, errors.WithStackTrace(diags)
	}

	return file, nil
}

func (parser *Parser) handleDiagnostics(file *File, diags hcl.Diagnostics) error {
	if len(diags) == 0 {
		return nil
	}

	if fn := parser.handleDiagnosticsFunc; fn != nil {
		var err error
		if diags, err = fn(file, diags); err != nil || diags == nil {
			return err
		}
	}

	if fn := parser.diagsWriterFunc; fn != nil {
		if err := fn(diags); err != nil {
			return err
		}
	}

	return diags
}
