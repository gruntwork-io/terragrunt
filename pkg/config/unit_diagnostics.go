package config

import (
	"errors"
	"slices"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// writtenDiagnostics is the diagnostics the assemblies of a parse wrote. The callers of a memoized parse share it.
type writtenDiagnostics struct {
	diags hcl.Diagnostics
	mu    sync.Mutex
}

// write writes the diagnostics of err that w has not yet written through file, and records them as written. An err
// that holds no [hcl.Diagnostics] writes nothing.
//
// Returns the error of the write, and err otherwise.
func (w *writtenDiagnostics) write(file *hclparse.File, err error) error {
	var diags hcl.Diagnostics
	if !errors.As(err, &diags) {
		return err
	}

	w.mu.Lock()
	fresh := excludeDiagnostics(diags, w.diags)
	w.diags = append(w.diags, fresh...)
	w.mu.Unlock()

	if writeErr := file.WriteDiagnostics(fresh); writeErr != nil {
		return writeErr
	}

	return err
}

// reportedDiagnostics returns err without the diagnostics that lie in a top-level argument or block of body that no
// section of decodeList decodes, as [withoutDiagnostics] does. The locals, include, and feature blocks are always
// reported. It returns err unchanged when decodeList is empty or err holds no [hcl.Diagnostics].
func reportedDiagnostics(decodeList []PartialDecodeSectionType, body hcl.Body, err error) error {
	var diags hcl.Diagnostics
	if len(decodeList) == 0 || !errors.As(err, &diags) {
		return err
	}

	skipped := newReportedItems(decodeList).skippedRanges(body)

	dropped := slices.DeleteFunc(slices.Clone(diags), func(diag *hcl.Diagnostic) bool {
		return diag.Subject == nil || !slices.ContainsFunc(skipped, func(rng hcl.Range) bool {
			return rng.Filename == diag.Subject.Filename && rng.ContainsOffset(diag.Subject.Start.Byte)
		})
	})

	return withoutDiagnostics(err, dropped)
}

// reportedItems is the top-level arguments and blocks whose diagnostics a partial parse reports.
type reportedItems struct {
	// names is the arguments and blocks reported whole.
	names map[string]struct{}
	// terraform is the arguments and blocks of the terraform block reported, nil when the block is reported whole or
	// not at all.
	terraform map[string]struct{}
}

// newReportedItems returns the items the sections of decodeList decode, with the base blocks.
func newReportedItems(decodeList []PartialDecodeSectionType) reportedItems {
	items := reportedItems{names: map[string]struct{}{
		MetadataLocals:      {},
		MetadataInclude:     {},
		MetadataFeatureFlag: {},
	}}

	for _, section := range decodeList {
		for _, name := range sectionItems(section) {
			items.names[name] = struct{}{}
		}
	}

	if slices.Contains(decodeList, TerraformBlock) {
		return items
	}

	if slices.Contains(decodeList, TerraformSource) || slices.Contains(decodeList, TerraformExtraArgs) {
		delete(items.names, MetadataTerraform)
		items.terraform = map[string]struct{}{"source": {}}
	}

	if slices.Contains(decodeList, TerraformExtraArgs) {
		items.terraform["extra_arguments"] = struct{}{}
	}

	return items
}

// sectionItems returns the top-level arguments and blocks section decodes. The terraform sections that decode part of
// the block name it whole, and [newReportedItems] narrows it.
func sectionItems(section PartialDecodeSectionType) []string {
	switch section {
	case DependenciesBlock:
		return []string{MetadataDependencies}
	case DependencyBlock:
		return []string{MetadataDependency}
	case TerraformBlock, TerraformSource, TerraformExtraArgs:
		return []string{MetadataTerraform}
	case TerragruntFlags:
		return []string{MetadataPreventDestroy, MetadataIamRole, MetadataIamWebIdentityToken}
	case TerragruntVersionConstraints:
		return []string{MetadataTerragruntVersionConstraint, MetadataTerraformVersionConstraint, MetadataTerraformBinary}
	case RemoteStateBlock:
		return []string{MetadataRemoteState}
	case FeatureFlagsBlock:
		return []string{MetadataFeatureFlag}
	case EngineBlock:
		return []string{MetadataEngine}
	case ExcludeBlock:
		return []string{MetadataExclude}
	case ErrorsBlock:
		return []string{MetadataErrors}
	case OutputsBlock:
		return []string{MetadataOutputs}
	}

	return nil
}

// skippedRanges returns the source ranges of the top-level arguments and blocks of body that i does not report, and
// of the parts of its terraform blocks i does not report. It skips no part of a body or terraform block whose content
// fails to list.
func (i reportedItems) skippedRanges(body hcl.Body) []hcl.Range {
	var skipped []hcl.Range

	for _, item := range namedRanges(body) {
		_, whole := i.names[item.name]
		if whole || item.name == MetadataTerraform && i.terraform != nil {
			continue
		}

		skipped = append(skipped, item.rng)
	}

	if i.terraform == nil {
		return skipped
	}

	content, _, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: MetadataTerraform}},
	})
	if diags.HasErrors() {
		return skipped
	}

	for _, block := range content.Blocks {
		for _, item := range namedRanges(block.Body) {
			if _, ok := i.terraform[item.name]; !ok {
				skipped = append(skipped, item.rng)
			}
		}
	}

	return skipped
}

// namedRange is the source range of an argument or block, by its name.
type namedRange struct {
	name string
	rng  hcl.Range
}

// namedRanges returns the source range of each argument and block of body. A body of HCL JSON gives each property as
// an argument, and none when its properties fail to list.
func namedRanges(body hcl.Body) []namedRange {
	syntax, ok := body.(*hclsyntax.Body)
	if !ok {
		attrs, diags := body.JustAttributes()
		if diags.HasErrors() {
			return nil
		}

		ranges := make([]namedRange, 0, len(attrs))
		for name, attr := range attrs {
			ranges = append(ranges, namedRange{name: name, rng: attr.Range})
		}

		return ranges
	}

	ranges := make([]namedRange, 0, len(syntax.Attributes)+len(syntax.Blocks))

	for name, attr := range syntax.Attributes {
		ranges = append(ranges, namedRange{name: name, rng: attr.SrcRange})
	}

	for _, block := range syntax.Blocks {
		ranges = append(ranges, namedRange{name: block.Type, rng: block.Range()})
	}

	return ranges
}
