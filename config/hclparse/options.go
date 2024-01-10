package hclparse

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/sirupsen/logrus"
)

type Option func(Parser) Parser

func WithLogger(logger *logrus.Entry) Option {
	return func(parser Parser) Parser {
		parser.logger = logger
		return parser
	}
}

// WithFileUpdate sets the `fileUpdateHandlerFunc` func which is run before each file decoding.
func WithFileUpdate(fn func(*File) error) Option {
	return func(parser Parser) Parser {
		parser.fileUpdateHandlerFunc = fn
		return parser
	}
}

// WithHaltOnErrorOnlyForBlocks configures a diagnostic error handler that runs when diagnostic errors occur.
// If errors occur in the given `blockNames` blocks, parser returns the error to its caller, otherwise it skips the error.
func WithHaltOnErrorOnlyForBlocks(blockNames []string) Option {
	return func(parser Parser) Parser {
		parser.diagnosticsErrorFunc = func(file *File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
			for _, sectionName := range blockNames {
				blocks, err := file.Blocks(sectionName, true)
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
		}

		return parser
	}
}
