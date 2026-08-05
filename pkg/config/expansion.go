package config

import (
	"slices"

	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// expansionCapableBlocks are the block types an expansion block may appear in.
var expansionCapableBlocks = []string{MetadataDependency, MetadataUnit, MetadataStack}

// ValidateExpansionExperiment rejects expansion blocks while the block-iteration
// experiment is disabled.
//
// This reads the raw body rather than the decoded config because unit and stack blocks
// decode through an `hcl:",remain"` field, which absorbs an unrecognized expansion block
// instead of rejecting it. Without this pass a user who writes expansion without the
// experiment watches the block silently do nothing.
//
// JSON configs parse into a body this cannot walk, so they fall through to the decoder's
// own unsupported-block error rather than the message naming the flag.
func ValidateExpansionExperiment(experiments experiment.Experiments, file *hclparse.File) error {
	if experiments.Evaluate(experiment.BlockIteration) {
		return nil
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	for _, block := range body.Blocks {
		if !slices.Contains(expansionCapableBlocks, block.Type) {
			continue
		}

		if !slices.ContainsFunc(block.Body.Blocks, func(inner *hclsyntax.Block) bool {
			return inner.Type == hclparse.ExpansionBlockName
		}) {
			continue
		}

		err := ExpansionRequiresExperimentError{
			ConfigPath: file.ConfigPath,
			BlockType:  block.Type,
		}
		if len(block.Labels) > 0 {
			err.BlockLabel = block.Labels[0]
		}

		return err
	}

	return nil
}
