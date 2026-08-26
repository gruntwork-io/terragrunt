package config

import (
	"slices"

	"github.com/hashicorp/hcl/v2"

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

// ValidateBlockIterationExperiment rejects block-iteration syntax while the experiment is
// off: an expansion block on a dependency, unit, or stack, and a bare enabled attribute on
// a unit or stack.
//
// This reads the raw body rather than the decoded config because unit and stack blocks
// decode through an `hcl:",remain"` field, which absorbs an unrecognized expansion block
// or attribute instead of rejecting it. Without this pass a user who writes expansion or
// enabled without the experiment watches the block silently do nothing.
func ValidateBlockIterationExperiment(experiments experiment.Experiments, file *hclparse.File) error {
	if experiments.Evaluate(experiment.BlockIteration) {
		return nil
	}

	// Diagnostics are dropped throughout this pass: a body malformed enough to produce them
	// fails the decode that follows with a message pointing at the real problem, and this
	// gate has nothing to add to it.
	content, _, _ := file.Body.PartialContent(blockIterationCandidateSchema)

	for _, block := range content.Blocks {
		label := ""
		if len(block.Labels) > 0 {
			label = block.Labels[0]
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
