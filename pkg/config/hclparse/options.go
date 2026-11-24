package hclparse

import (
	"io"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
)

// Option configures a [Parser].
type Option func(*Parser) *Parser

// WithLogger sets the logger the parser warns through when reading or parsing a file fails.
func WithLogger(logger log.Logger) Option {
	return func(parser *Parser) *Parser {
		parser.logger = logger

		return parser
	}
}

// WithDiagnosticsWriter writes diagnostics that contain an error to writer, formatted for the terminal v describes.
func WithDiagnosticsWriter(v *venv.Venv, writer io.Writer, disableColor bool) Option {
	return func(parser *Parser) *Parser {
		diagsWriter := parser.GetDiagnosticsWriter(v, writer, disableColor)

		parser.diagsWriterFunc = func(diags hcl.Diagnostics) error {
			if !diags.HasErrors() {
				return nil
			}

			if err := diagsWriter.WriteDiagnostics(diags); err != nil {
				return err
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
		parser.handleDiagnosticsFunc = appendHandleDiagnosticsFunc(
			parser.handleDiagnosticsFunc,
			func(file *File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
				if file == nil || !diags.HasErrors() {
					return diags, nil
				}

				for _, blockName := range blockNames {
					blocks, err := file.Blocks(blockName, true)
					if err != nil {
						return nil, err
					}

					for _, block := range blocks {
						blockAttrs, attrDiags := block.Body.JustAttributes()
						if attrDiags.HasErrors() {
							return diags, nil
						}

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
			},
		)

		return parser
	}
}

// WithDiagnosticsHandler adds fn to the diagnostics handlers, running it after any handler set before. fn returns
// the diagnostics to keep: nil drops them, and an error replaces them.
func WithDiagnosticsHandler(
	fn func(file *hcl.File, diags hcl.Diagnostics) (hcl.Diagnostics, error),
) Option {
	return func(parser *Parser) *Parser {
		parser.handleDiagnosticsFunc = appendHandleDiagnosticsFunc(
			parser.handleDiagnosticsFunc,
			func(file *File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
				return fn(file.File, diags)
			},
		)

		return parser
	}
}

func appendHandleDiagnosticsFunc(
	prev, next func(*File, hcl.Diagnostics) (hcl.Diagnostics, error),
) func(*File, hcl.Diagnostics) (hcl.Diagnostics, error) {
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
