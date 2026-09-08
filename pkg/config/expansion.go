package config

import (
	"slices"
	"strings"

	"github.com/agext/levenshtein"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

const enabledAttrName = "enabled"

// enabledGatedBlocks omits dependency, which accepted a bare enabled attribute long
// before this experiment existed.
var enabledGatedBlocks = []string{MetadataUnit, MetadataStack}

// blockIterationCandidateSchema matches every block type block-iteration syntax may appear in.
// All three carry a single name label.
var blockIterationCandidateSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: MetadataDependency, LabelNames: []string{"name"}},
		{Type: MetadataUnit, LabelNames: []string{"name"}},
		{Type: MetadataStack, LabelNames: []string{"name"}},
	},
}

// blockIterationSyntaxSchema matches the two pieces of block-iteration syntax a candidate
// block may carry.
var blockIterationSyntaxSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{{Name: enabledAttrName}},
	Blocks:     []hcl.BlockHeaderSchema{{Type: hclparse.ExpansionBlockName}},
}

// maxExpansionTypoDistance is how far a nested block name may sit from expansion and still
// read as a misspelling of it. Two edits cover a dropped, doubled, or swapped letter. The
// only other block a unit or stack nests, autoinclude, sits well outside that.
const maxExpansionTypoDistance = 2

// ValidateBlockIteration rejects block-iteration syntax that Terragrunt would otherwise run
// without complaint: syntax the block-iteration experiment gates while the experiment is
// off, and a misspelling of the expansion block name.
//
// This reads the raw body rather than the decoded config because unit and stack blocks
// decode through an `hcl:",remain"` field, which absorbs an unrecognized block or attribute
// instead of rejecting it. Without this pass a user who writes expansion without the
// experiment, or writes expanson with it, watches the block silently do nothing.
func ValidateBlockIteration(experiments experiment.Experiments, file *hclparse.File) error {
	// Diagnostics are dropped throughout this pass: a body malformed enough to produce them
	// fails the decode that follows with a message pointing at the real problem, and this
	// gate has nothing to add to it.
	content, _, _ := file.Body.PartialContent(blockIterationCandidateSchema)

	iterationEnabled := experiments.Evaluate(experiment.BlockIteration)

	for _, block := range content.Blocks {
		label := ""
		if len(block.Labels) > 0 {
			label = block.Labels[0]
		}

		if name := misspelledExpansionBlock(block); name != "" {
			return MisspelledExpansionBlockError{
				ConfigPath: file.ConfigPath,
				BlockType:  block.Type,
				BlockLabel: label,
				BlockName:  name,
			}
		}

		if iterationEnabled {
			continue
		}

		inner, _, _ := block.Body.PartialContent(blockIterationSyntaxSchema)

		if len(inner.Blocks) > 0 {
			return ExpansionRequiresExperimentError{
				ConfigPath: file.ConfigPath,
				BlockType:  block.Type,
				BlockLabel: label,
			}
		}

		if !slices.Contains(enabledGatedBlocks, block.Type) {
			continue
		}

		if _, declared := inner.Attributes[enabledAttrName]; declared {
			return EnabledRequiresExperimentError{
				ConfigPath: file.ConfigPath,
				BlockType:  block.Type,
				BlockLabel: label,
			}
		}
	}

	return nil
}

// misspelledExpansionBlock returns the name of the first block nested inside block that sits
// close enough to expansion to read as a typo of it, and the empty string when none does. A
// name differing only in case counts, since the decoder matches block names exactly.
//
// Only native HCL syntax is read. A JSON body draws no line between a nested block and an
// attribute, so it offers no block name to compare.
func misspelledExpansionBlock(block *hcl.Block) string {
	body, ok := block.Body.(*hclsyntax.Body)
	if !ok {
		return ""
	}

	for _, nested := range body.Blocks {
		if nested.Type == hclparse.ExpansionBlockName {
			continue
		}

		distance := levenshtein.Distance(
			strings.ToLower(nested.Type),
			hclparse.ExpansionBlockName,
			nil,
		)
		if distance <= maxExpansionTypoDistance {
			return nested.Type
		}
	}

	return ""
}
