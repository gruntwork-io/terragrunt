// Package hclparse provides a wrapper around the HCL2 parser to handle diagnostics and errors in a more user-friendly way.
//
// The package wraps `hclparse.Parser` to be able to handle diagnostic errors from one place, see `handleDiagnostics(diags hcl.Diagnostics) error` func.
// This allows us to halt the process only when certain errors occur, such as skipping all errors not related to the `catalog` block.
package hclparse

import (
	"io"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

type Parser struct {
	*hclparse.Parser
	diagsWriterFunc       func(hcl.Diagnostics) error
	handleDiagnosticsFunc func(*File, hcl.Diagnostics) (hcl.Diagnostics, error)
	fileUpdateHandlerFunc func(*File) error
	logger                log.Logger
}

func NewParser(opts ...Option) *Parser {
	return (&Parser{
		Parser: hclparse.NewParser(),
		logger: log.Default(),
	}).withOptions(opts...)
}

func (parser *Parser) withOptions(opts ...Option) *Parser {
	for _, opt := range opts {
		parser = opt(parser)
	}

	return parser
}

// ParseFromFile reads configPath from fsys and parses it into an HCL file body.
func (parser *Parser) ParseFromFile(fsys vfs.FS, configPath string) (*File, error) {
	content, err := vfs.ReadFile(fsys, configPath)
	if err != nil {
		parser.logger.Warnf("Error reading file %s: %v", configPath, err)

		return nil, err
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
			err = PanicWhileParsingConfigError{RecoveredValue: recovered, ConfigFile: configPath}
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
		diags = append(diags, rejectOutOfRangeNumbers(hclFile)...)
	}

	file = &File{
		Parser:     parser,
		File:       hclFile,
		ConfigPath: configPath,
	}

	if err := parser.handleDiagnostics(file, diags); err != nil {
		parser.logger.Warnf("Failed to parse HCL in file %s: %v", configPath, diags)

		return nil, diags
	}

	return file, nil
}

// GetDiagnosticsWriter returns a hcl2 parsing diagnostics emitter for the
// terminal v describes. A run with no terminal gets width 0, which disables
// word-wrapping: wrapping there would split messages at positions that shift
// with path lengths, which is awkward to read back and to test against.
func (parser *Parser) GetDiagnosticsWriter(
	v *venv.Venv,
	writer io.Writer,
	disableColor bool,
) hcl.DiagnosticWriter {
	v.RequireTerminal()

	termColor := !disableColor && v.Terminal.StderrIsTTY()

	return hcl.NewDiagnosticTextWriter(writer, parser.Files(), uint(v.Terminal.Width()), termColor)
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

// rejectOutOfRangeNumbers reports each number literal in file whose magnitude is outside the
// range [ctyhelper.ValidateNumberRanges] allows, and makes its value unknown. Converting such a
// number to a string, as toset(["a", 1e999999999]) or "${1e999999999}" do, writes out every
// digit, which takes minutes. The literal is disarmed as well as reported because some callers
// carry on evaluating a config that failed to parse.
//
// The parser never produces an unknown literal, so one found here was disarmed by an earlier
// call and is reported again. That matters because [hclparse.Parser.ParseHCL] hands back the
// file it cached for a path it has already parsed, without the original diagnostics.
//
// file must come from [hclparse.Parser.ParseHCL], which always returns an [hclsyntax.Body].
func rejectOutOfRangeNumbers(file *hcl.File) hcl.Diagnostics {
	return hclsyntax.VisitAll(
		file.Body.(*hclsyntax.Body),
		func(node hclsyntax.Node) hcl.Diagnostics {
			lit, ok := node.(*hclsyntax.LiteralValueExpr)
			if !ok {
				return nil
			}

			err := ctyhelper.ValidateNumberRanges(lit.Val)
			if err == nil && lit.Val.IsKnown() {
				return nil
			}

			if err == nil {
				err = ctyhelper.NumberOutOfRangeError{}
			}

			lit.Val = cty.DynamicVal

			return hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Number out of range",
				Detail:   err.Error(),
				Subject:  lit.SrcRange.Ptr(),
			}}
		},
	)
}
