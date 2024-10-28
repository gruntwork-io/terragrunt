//go:generate mockery --name Provider

package getproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/slices"
)

// UpdateLockfile updates the dependency lock file. If `.terraform.lock.hcl` does not exist, it will be created, otherwise it will be updated.
func UpdateLockfile(ctx context.Context, workingDir string, providers []Provider) error {
	var (
		filename = filepath.Join(workingDir, terraform.TerraformLockFile)
		file     = hclwrite.NewFile()
	)

	if util.FileExists(filename) {
		content, err := os.ReadFile(filename)
		if err != nil {
			return errors.New(err)
		}

		var diags hcl.Diagnostics

		file, diags = hclwrite.ParseConfig(content, filename, hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return errors.New(diags)
		}
	}

	if err := updateLockfile(ctx, file, providers); err != nil {
		return err
	}

	const ownerWriteGlobalReadPerms = 0644
	if err := os.WriteFile(filename, file.Bytes(), ownerWriteGlobalReadPerms); err != nil {
		return errors.New(err)
	}

	return nil
}

func updateLockfile(ctx context.Context, file *hclwrite.File, providers []Provider) error {
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Address() < providers[j].Address()
	})

	for _, provider := range providers {
		providerBlock := file.Body().FirstMatchingBlock("provider", []string{provider.Address()})
		if providerBlock != nil {
			// update the existing provider block
			if err := updateProviderBlock(ctx, providerBlock, provider); err != nil {
				return err
			}
		} else {
			// create a new provider block
			file.Body().AppendNewline()
			providerBlock = file.Body().AppendNewBlock("provider", []string{provider.Address()})

			if err := updateProviderBlock(ctx, providerBlock, provider); err != nil {
				return err
			}
		}
	}

	return nil
}

// updateProviderBlock updates the provider block in the dependency lock file.
func updateProviderBlock(ctx context.Context, providerBlock *hclwrite.Block, provider Provider) error {
	hashes, err := getExistingHashes(providerBlock, provider)
	if err != nil {
		return err
	}

	providerBlock.Body().SetAttributeValue("version", cty.StringVal(provider.Version()))

	// Constraints can contain multiple constraint expressions, including comparison operators, but in the Terragrunt Provider Cache use case, we assume that the required_providers are pinned to a specific version to detect the required version without terraform init, so we can simply specify the constraints attribute as the same as the version. This may differ from what terraform generates, but we expect that it doesn't matter in practice.
	providerBlock.Body().SetAttributeValue("constraints", cty.StringVal(provider.Version()))

	h1Hash, err := PackageHashV1(provider.PackageDir())
	if err != nil {
		return err
	}

	newHashes := []Hash{h1Hash}

	documentSHA256Sums, err := provider.DocumentSHA256Sums(ctx)
	if err != nil {
		return err
	}

	if documentSHA256Sums != nil {
		zipHashes := DocumentHashes(documentSHA256Sums)
		newHashes = append(newHashes, zipHashes...)
	}

	// merge with existing hashes
	for _, newHashe := range newHashes {
		if !util.ListContainsElement(hashes, newHashe) {
			hashes = append(hashes, newHashe)
		}
	}

	slices.Sort(hashes)

	providerBlock.Body().SetAttributeRaw("hashes", tokensForListPerLine(hashes))

	return nil
}

func getExistingHashes(providerBlock *hclwrite.Block, provider Provider) ([]Hash, error) {
	versionAttr := providerBlock.Body().GetAttribute("version")
	if versionAttr == nil {
		return nil, nil
	}

	var hashes []Hash

	// a version attribute found
	versionVal := getAttributeValueAsUnquotedString(versionAttr)
	provider.Logger().Debugf("Check provider version in lock file: address = %s, lock = %s, config = %s", provider.Address(), versionVal, provider.Version())

	if versionVal == provider.Version() {
		// if version is equal, get already existing hashes from lock file to merge.
		if attr := providerBlock.Body().GetAttribute("hashes"); attr != nil {
			vals, err := getAttributeValueAsSlice(attr)
			if err != nil {
				return nil, err
			}

			for _, val := range vals {
				hashes = append(hashes, Hash(val))
			}
		}
	}

	return hashes, nil
}

// getAttributeValueAsString returns a value of Attribute as string. There is no way to get value as string directly, so we parses tokens of Attribute and build string representation.
func getAttributeValueAsUnquotedString(attr *hclwrite.Attribute) string {
	// find TokenEqual
	expr := attr.Expr()
	exprTokens := expr.BuildTokens(nil)

	// TokenIdent records SpaceBefore, but we should ignore it here.
	quotedValue := strings.TrimSpace(string(exprTokens.Bytes()))

	// unquote
	value := strings.Trim(quotedValue, "\"")

	return value
}

// getAttributeValueAsSlice returns a value of Attribute as slice.
func getAttributeValueAsSlice(attr *hclwrite.Attribute) ([]string, error) {
	expr := attr.Expr()
	exprTokens := expr.BuildTokens(nil)

	valBytes := bytes.TrimFunc(exprTokens.Bytes(), func(r rune) bool {
		if unicode.IsSpace(r) || r == ']' || r == ',' {
			return true
		}

		return false
	})
	valBytes = append(valBytes, ']')

	var val []string

	if err := json.Unmarshal(valBytes, &val); err != nil {
		return nil, errors.New(err)
	}

	return val, nil
}

// tokensForListPerLine builds a hclwrite.Tokens for a given hashes, but breaks the line for each element.
func tokensForListPerLine(hashes []Hash) hclwrite.Tokens {
	// The original TokensForValue implementation does not break line by line for hashes, so we build a token sequence by ourselves.
	tokens := append(hclwrite.Tokens{},
		&hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte{'['}},
		&hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte{'\n'}})

	for _, hash := range hashes {
		ts := hclwrite.TokensForValue(cty.StringVal(hash.String()))
		tokens = append(tokens, ts...)
		tokens = append(tokens,
			&hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte{','}},
			&hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte{'\n'}})
	}

	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte{']'}})

	return tokens
}
