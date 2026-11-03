package config

import (
	"strings"

	"github.com/agext/levenshtein"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// expandableBlockTypes holds every block type that may nest an expansion block. All three
// carry a single name label.
var expandableBlockTypes = map[string]struct{}{
	MetadataDependency: {},
	MetadataUnit:       {},
	MetadataStack:      {},
}

// maxExpansionTypoDistance is how far a nested block name may sit from expansion and still
// read as a misspelling of it. Two edits cover a dropped, doubled, or swapped letter. The
// only other block a unit or stack nests, autoinclude, sits well outside that.
const maxExpansionTypoDistance = 2

// ValidateExpansionSpelling rejects a dependency, unit, or stack block that nests a misspelling
// of the expansion block name, which Terragrunt would otherwise run without complaint.
//
// This reads the raw body rather than the decoded config because unit and stack blocks
// decode through an `hcl:",remain"` field, which absorbs an unrecognized block instead of
// rejecting it. Without this pass a user who writes expanson watches the block silently do
// nothing.
//
// Only native HCL syntax is read. A JSON body draws no line between a nested block and an
// attribute, so it offers no block name to compare. A block without exactly one label is
// skipped, since the decode that follows rejects it with a message pointing at the real
// problem.
func ValidateExpansionSpelling(file *hclparse.File) error {
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	for _, block := range body.Blocks {
		if _, ok := expandableBlockTypes[block.Type]; !ok || len(block.Labels) != 1 {
			continue
		}

		name := misspelledExpansionBlock(block.Body)
		if name == "" {
			continue
		}

		return MisspelledExpansionBlockError{
			ConfigPath: file.ConfigPath,
			BlockType:  block.Type,
			BlockLabel: block.Labels[0],
			BlockName:  name,
		}
	}

	return nil
}

// misspelledExpansionBlock returns the name of the first block nested inside body that sits
// close enough to expansion to read as a typo of it, and the empty string when none does. A
// name differing only in case counts, since the decoder matches block names exactly.
func misspelledExpansionBlock(body *hclsyntax.Body) string {
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
