package hclparse

import (
	"io"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
)

type Option func(*Parser) *Parser

func WithLogger(logger log.Logger) Option {
	return func(parser *Parser) *Parser {
		parser.logger = logger

		return parser
	}
}

func WithDiagnosticsWriter(writer io.Writer, disableColor bool) Option {
	return func(parser *Parser) *Parser {
		diagsWriter := parser.GetDiagnosticsWriter(writer, disableColor)

		parser.diagsWriterFunc = func(diags hcl.Diagnostics) error {
			if !diags.HasErrors() {
				return nil
			}

			if err := diagsWriter.WriteDiagnostics(diags); err != nil {
				return errors.New(err)
			}

			return nil
		}

		return parser
	}
}

// WithFileUpdate sets the `fileUpdateHandlerFunc` func which is run before each file decoding.
func WithFileUpdate(fn func(*File) error) Option {
	return func(parser *Parser) *Parser {
		parser.fileUpdateHandlerFunc = fn
		return parser
	}
}

// WithHaltOnErrorOnlyForBlocks configures a diagnostic error handler that runs when diagnostic errors occur.
// If errors occur in the given `blockNames` blocks, parser returns the error to its caller, otherwise it skips the error.
func WithHaltOnErrorOnlyForBlocks(blockNames []string) Option {
	return func(parser *Parser) *Parser {
		parser.handleDiagnosticsFunc = appendHandleDiagnosticsFunc(parser.handleDiagnosticsFunc, func(file *File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
			if file == nil || !diags.HasErrors() {
				return diags, nil
			}

			for _, blockName := range blockNames {
				blocks, err := file.Blocks(blockName, true)
				if err != nil {
					return nil, err
				}

				for _, block := range blocks {
					blockAttrs, _ := block.Body.JustAttributes()

					for _, blokcAttr := range blockAttrs {
						for _, diag := range diags {
							if diag.Context != nil && blokcAttr.Range.Overlaps(*diag.Context) {
								return diags, nil
							}
						}
					}
				}
			}

			return nil, nil
		})

		return parser
	}
}

func WithDiagnosticsHandler(fn func(file *hcl.File, diags hcl.Diagnostics) (hcl.Diagnostics, error)) Option {
	return func(parser *Parser) *Parser {
		parser.handleDiagnosticsFunc = appendHandleDiagnosticsFunc(parser.handleDiagnosticsFunc, func(file *File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
			return fn(file.File, diags)
		})

		return parser
	}
}

func appendHandleDiagnosticsFunc(prev, next func(*File, hcl.Diagnostics) (hcl.Diagnostics, error)) func(*File, hcl.Diagnostics) (hcl.Diagnostics, error) {
	return func(file *File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
		var err error

		if prev != nil {
			if diags, err = prev(file, diags); err != nil {
				return diags, err
			}
		}

		return next(file, diags)
	}
}
