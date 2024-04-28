package getproviders

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/slices"
)

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

// UpdateLockfile updates the dependency lock file.
func updateLockfile(ctx context.Context, file *hclwrite.File, providers Providers) error {
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

	documentSHA256Sums, err := provider.DocumentSHA256Sums(ctx)
	if err != nil {
		return err
	}

	h1Hash, err := PackageHashV1(provider.PackageDir())
	if err != nil {
		return err
	}

	hashes := append(DocumentHashes(documentSHA256Sums), h1Hash)
	slices.Sort(hashes)

	providerBlock.Body().SetAttributeRaw("hashes", tokensForListPerLine(hashes))
	return nil
}
