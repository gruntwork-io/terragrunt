package hclparse

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
)

const (
	// A consistent error message for multiple catalog block in terragrunt config (which is currently not supported)
	multipleBlockDetailFmt = "Terragrunt currently does not support multiple %[1]s blocks in a single config. Consolidate to a single %[1]s block."
)

type File struct {
	*Parser
	*hcl.File
	ConfigPath string
}

func (file *File) Content() string {
	return string(file.Bytes)
}

// Update reparses the file with the new `content`.
func (file *File) Update(content []byte) error {
	// Since `hclparse.Parser` has a cache, we need to recreate(clone) the Parser instance without current file
	// to be able to parse the configuration with the same `configPath`.
	parser := hclparse.NewParser()

	for configPath, copyfile := range file.Files() {
		if configPath != file.ConfigPath {
			parser.AddFile(configPath, copyfile)
		}
	}

	file.Parser.Parser = parser

	// we need to reparse the new updated contents. This is necessarily because the blocks
	// returned by hclparse does not support editing, and so we have to go through hclwrite, which leads to a
	// different AST representation.
	updatedFile, err := file.ParseFromBytes(content, file.ConfigPath)
	if err != nil {
		return err
	}

	file.File = updatedFile.File

	return nil
}

// Decode uses the HCL2 parser to decode the parsed HCL into the struct specified by out.
//
// Note that we take a two pass approach to support parsing include blocks without a label. Ideally we can parse include
// blocks with and without labels in a single pass, but the HCL parser is fairly restrictive when it comes to parsing
// blocks with labels, requiring the exact number of expected labels in the parsing step.  To handle this restriction,
// we first see if there are any include blocks without any labels, and if there is, we modify it in the file object to
// inject the label as "".
func (file *File) Decode(out interface{}, evalContext *hcl.EvalContext) (err error) {
	if file.fileUpdateHandlerFunc != nil {
		if err := file.Parser.fileUpdateHandlerFunc(file); err != nil {
			return err
		}
	}

	diags := gohcl.DecodeBody(file.Body, evalContext, out)
	if err := file.HandleDiagnostics(diags); err != nil {
		return errors.New(err)
	}

	return nil
}

// Blocks takes a parsed HCL file and extracts a reference to the `name` block, if there are defined.
func (file *File) Blocks(name string, isMultipleAllowed bool) ([]*Block, error) {
	catalogSchema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: name},
		},
	}
	// We use PartialContent here, because we are only interested in parsing out the catalog block.
	parsed, _, diags := file.Body.PartialContent(catalogSchema)
	if err := file.HandleDiagnostics(diags); err != nil {
		return nil, errors.New(err)
	}

	extractedBlocks := []*Block{}

	for _, block := range parsed.Blocks {
		if block.Type == name {
			extractedBlocks = append(extractedBlocks, &Block{
				File:  file,
				Block: block,
			})
		}
	}

	if len(extractedBlocks) > 1 && !isMultipleAllowed {
		return nil, errors.New(
			&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  fmt.Sprintf("Multiple %s block", name),
				Detail:   fmt.Sprintf(multipleBlockDetailFmt, name),
			},
		)
	}

	return extractedBlocks, nil
}

func (file *File) JustAttributes() (Attributes, error) {
	hclAttrs, diags := file.Body.JustAttributes()

	if err := file.HandleDiagnostics(diags); err != nil {
		return nil, errors.New(err)
	}

	attrs := NewAttributes(file, hclAttrs)

	if err := attrs.ValidateIdentifier(); err != nil {
		return nil, err
	}

	return attrs, nil
}

func (file *File) HandleDiagnostics(diags hcl.Diagnostics) error {
	return file.Parser.handleDiagnostics(file, diags)
}
