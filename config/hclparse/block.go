package hclparse

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/hashicorp/hcl/v2"
)

// Detailed error messages in diagnostics returned by parsing locals
const (
	// A consistent detail message for all "not a valid identifier" diagnostics. This is exactly the same as that returned
	// by terraform.
	badIdentifierDetail = "A name must start with a letter and may contain only letters, digits, underscores, and dashes."
)

type Block struct {
	*File
	*hcl.Block
}

// JustAttributes loads the block into name expression pairs to assist with evaluation of the attrs prior to
// evaluating the whole config. Note that this is exactly the same as
// terraform/configs/named_values.go:decodeLocalsBlock
func (block *Block) JustAttributes() (Attributes, error) {
	hclAttrs, diags := block.Body.JustAttributes()

	if err := block.HandleDiagnostics(diags); err != nil {
		return nil, errors.New(err)
	}

	attrs := NewAttributes(block.File, hclAttrs)

	if err := attrs.ValidateIdentifier(); err != nil {
		return nil, err
	}

	return attrs, nil
}
