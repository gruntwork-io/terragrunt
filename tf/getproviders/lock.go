//go:generate mockgen -source=$GOFILE -destination=mocks/mock_$GOFILE -package=mocks

package getproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// UpdateLockfile updates the dependency lock file. If `.terraform.lock.hcl` does not exist, it will be created, otherwise it will be updated.
func UpdateLockfile(ctx context.Context, workingDir string, providers []Provider) error {
	var (
		filename = filepath.Join(workingDir, tf.TerraformLockFile)
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

	// If version constraints exist in current lock file and match the new version, we keep them unchanged.
	// Otherwise, we specify the constraints attribute the same as the version.
	currentConstraintsAttr := providerBlock.Body().GetAttribute("constraints")

	shouldUpdateConstraints := false

	if currentConstraintsAttr != nil {
		currentConstraintsValue := strings.ReplaceAll(string(currentConstraintsAttr.Expr().BuildTokens(nil).Bytes()), `"`, "")
		currentConstraints, err := version.NewConstraint(currentConstraintsValue)
		// If current version constraints are malformed, we should update it.
		if err != nil {
			shouldUpdateConstraints = true
		} else {
			newVersion, _ := version.NewVersion(provider.Version())
			// If current version constrains do not match the new provider version, we should update it.
			if !currentConstraints.Check(newVersion) {
				shouldUpdateConstraints = true
			} else {
				// Even if current constraints are valid, check if module constraints have changed
				moduleConstraints := provider.Constraints()
				if moduleConstraints != "" && moduleConstraints != currentConstraintsValue {
					shouldUpdateConstraints = true
				}
			}
		}
	} else {
		// If there is no constraints attribute, we should update it.
		shouldUpdateConstraints = true
	}

	if shouldUpdateConstraints {
		// Use module constraints if available, otherwise fall back to exact version
		constraintsValue := provider.Constraints()
		if constraintsValue == "" {
			constraintsValue = provider.Version()
		}

		providerBlock.Body().SetAttributeValue("constraints", cty.StringVal(constraintsValue))
	}

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

// UpdateLockfileConstraints updates only the constraints in an existing lock file
// This is used for upgrade scenarios where module constraints have changed
// but no providers were newly downloaded
func UpdateLockfileConstraints(ctx context.Context, workingDir string, constraints ProviderConstraints) error {
	filename := filepath.Join(workingDir, tf.TerraformLockFile)

	if !util.FileExists(filename) {
		return nil // No lock file to update
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		return errors.New(err)
	}

	file, diags := hclwrite.ParseConfig(content, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return errors.New(diags)
	}

	updated := false

	// Update constraints for each provider in the lock file
	for providerAddr, newConstraint := range constraints {
		providerBlock := file.Body().FirstMatchingBlock("provider", []string{providerAddr})
		if providerBlock != nil {
			currentConstraintsAttr := providerBlock.Body().GetAttribute("constraints")
			if currentConstraintsAttr != nil {
				currentConstraintsValue := strings.ReplaceAll(string(currentConstraintsAttr.Expr().BuildTokens(nil).Bytes()), `"`, "")
				if currentConstraintsValue != newConstraint {
					providerBlock.Body().SetAttributeValue("constraints", cty.StringVal(newConstraint))

					updated = true
				}
			}
		}
	}

	if updated {
		const ownerWriteGlobalReadPerms = 0644
		if err := os.WriteFile(filename, file.Bytes(), ownerWriteGlobalReadPerms); err != nil {
			return errors.New(err)
		}
	}

	return nil
}
