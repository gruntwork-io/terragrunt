package hclparser

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
func WithFileUpdate(fn func(*File) error) Option {
	return func(parser Parser) Parser {
		parser.fileUpdateHandlerFunc = fn
		return parser
	}
}

func WithHaltOnErrorOnlyForSections(sectionNames []string) Option {
	return func(parser Parser) Parser {
		parser.diagnosticsErrorFunc = func(file *File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
			for _, sectionName := range sectionNames {
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
