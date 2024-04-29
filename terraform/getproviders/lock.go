package getproviders

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/slices"
)

// UpdateLockfile updates the dependency lock file. If `.terraform.lock.hcl` does not exist, it will be created, otherwise it will be updated.
func UpdateLockfile(ctx context.Context, workingDir string, providers Providers) error {
	var (
		filename = filepath.Join(workingDir, terraform.TerraformLockFile)
		file     = hclwrite.NewFile()
	)

	if util.FileExists(filename) {
		content, err := os.ReadFile(filename)
		if err != nil {
			return errors.WithStackTrace(err)
		}

		var diags hcl.Diagnostics
		file, diags = hclwrite.ParseConfig(content, filename, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return errors.WithStackTrace(diags)
		}
	}

	if err := updateLockfile(ctx, file, providers); err != nil {
		return err
	}

	if err := os.WriteFile(filename, file.Bytes(), 0644); err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

func updateLockfile(ctx context.Context, file *hclwrite.File, providers Providers) error {
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Address() < providers[j].Address()
	})

	for _, provider := range providers {
		providerBlock := file.Body().FirstMatchingBlock("provider", []string{provider.Address()})
		if providerBlock != nil {
			// update the existing provider block
			err := updateProviderBlock(ctx, providerBlock, provider)
			if err != nil {
				return err
			}
		} else {
			// create a new provider block
			file.Body().AppendNewline()
			providerBlock = file.Body().AppendNewBlock("provider", []string{provider.Address()})

			err := updateProviderBlock(ctx, providerBlock, provider)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// updateProviderBlock updates the provider block in the dependency lock file.
func updateProviderBlock(ctx context.Context, providerBlock *hclwrite.Block, provider Provider) error {
	versionAttr := providerBlock.Body().GetAttribute("version")
	if versionAttr != nil {
		// a version attribute found
		versionVal := getAttributeValueAsUnquotedString(versionAttr)
		log.Debugf("Check provider version in lock file: address = %s, lock = %s, config = %s", provider.Address(), versionVal, provider.Version())
		if versionVal == provider.Version() {
			// Avoid unnecessary recalculations if no version change
			return nil
		}
	}

	providerBlock.Body().SetAttributeValue("version", cty.StringVal(provider.Version()))

	// Constraints can contain multiple constraint expressions, including comparison operators, but in the Terragrunt Provider Cache use case, we assume that the required_providers are pinned to a specific version to detect the required version without terraform init, so we can simply specify the constraints attribute as the same as the version. This may differ from what terraform generates, but we expect that it doesn't matter in practice.
	providerBlock.Body().SetAttributeValue("constraints", cty.StringVal(provider.Version()))

	documentSHA256Sums, err := provider.DocumentSHA256Sums(ctx)
	if err != nil {
		return err
	}

	h1Hash, err := PackageHashV1(provider.PackageDir())
	if err != nil {
		return err
	}
	zipHashes := DocumentHashes(documentSHA256Sums)

	hashes := append(zipHashes, h1Hash)
	slices.Sort(hashes)

	providerBlock.Body().SetAttributeRaw("hashes", tokensForListPerLine(hashes))
	return nil
}
