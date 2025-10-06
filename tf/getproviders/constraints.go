package getproviders

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// ProviderConstraints maps provider addresses to their version constraints from required_providers blocks
type ProviderConstraints map[string]string

// ParseProviderConstraints parses all .tf and .tofu files in the given directory and extracts required_providers constraints
func ParseProviderConstraints(opts *options.TerragruntOptions, workingDir string) (ProviderConstraints, error) {
	constraints := make(ProviderConstraints)

	var allFiles []string

	tfFiles, err := filepath.Glob(filepath.Join(workingDir, "*.tf"))
	if err != nil {
		return nil, errors.New(err)
	}

	allFiles = append(allFiles, tfFiles...)

	tofuFiles, err := filepath.Glob(filepath.Join(workingDir, "*.tofu"))
	if err != nil {
		return nil, errors.New(err)
	}

	allFiles = append(allFiles, tofuFiles...)

	// If no terraform files found, return empty constraints (not an error)
	if len(allFiles) == 0 {
		return constraints, nil
	}

	for _, file := range allFiles {
		fileConstraints, err := parseProviderConstraintsFromFile(opts, file)
		if err != nil {
			// Log parsing errors but continue processing other files
			// This allows partial success when some files have syntax errors
			continue
		}

		// Merge constraints from this file
		maps.Copy(constraints, fileConstraints)
	}

	return constraints, nil
}

// parseProviderConstraintsFromFile parses a single .tf file and extracts required_providers constraints
func parseProviderConstraintsFromFile(opts *options.TerragruntOptions, filename string) (ProviderConstraints, error) {
	constraints := make(ProviderConstraints)

	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.New(err)
	}

	// Parse the HCL file
	file, diags := hclsyntax.ParseConfig(content, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, errors.New(diags)
	}

	// Walk through the file looking for terraform blocks with required_providers
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, errors.New("failed to parse HCL body")
	}

	for _, block := range body.Blocks {
		if block.Type != "terraform" {
			continue
		}

		// Look for required_providers block within terraform block
		for _, nestedBlock := range block.Body.Blocks {
			if nestedBlock.Type != "required_providers" {
				continue
			}

			// Parse each provider in the required_providers block
			providerConstraints := parseProvidersFromRequiredProvidersBlock(opts, nestedBlock)

			// Merge constraints from this required_providers block
			maps.Copy(constraints, providerConstraints)
		}
	}

	return constraints, nil
}

// parseProvidersFromRequiredProvidersBlock extracts provider constraints from a required_providers block
func parseProvidersFromRequiredProvidersBlock(opts *options.TerragruntOptions, block *hclsyntax.Block) ProviderConstraints {
	constraints := make(ProviderConstraints)

	// Parse the attributes in the required_providers block
	for name, attr := range block.Body.Attributes {
		// Skip if not an object expression (should be provider configuration)
		objExpr, ok := attr.Expr.(*hclsyntax.ObjectConsExpr)
		if !ok {
			continue
		}

		var source, version string

		// Extract source and version from the provider configuration
		for _, item := range objExpr.Items {
			keyExpr, ok := item.KeyExpr.(*hclsyntax.ObjectConsKeyExpr)
			if !ok {
				continue
			}

			// Get the key name
			keyName := ""

			if keyExpr.Wrapped != nil {
				// Try different types of key expressions
				switch expr := keyExpr.Wrapped.(type) {
				case *hclsyntax.TemplateExpr:
					if len(expr.Parts) == 1 {
						if literal, ok := expr.Parts[0].(*hclsyntax.LiteralValueExpr); ok {
							keyName = literal.Val.AsString()
						}
					}
				case *hclsyntax.ScopeTraversalExpr:
					// This handles bare identifiers like "source" or "version"
					if len(expr.Traversal) == 1 {
						if root, ok := expr.Traversal[0].(hcl.TraverseRoot); ok {
							keyName = root.Name
						}
					}
				case *hclsyntax.LiteralValueExpr:
					// Direct literal value
					if expr.Val.Type() == cty.String {
						keyName = expr.Val.AsString()
					}
				}
			}

			// Get the value
			var value string

			if templateExpr, ok := item.ValueExpr.(*hclsyntax.TemplateExpr); ok {
				if len(templateExpr.Parts) == 1 {
					if literal, ok := templateExpr.Parts[0].(*hclsyntax.LiteralValueExpr); ok {
						if literal.Val.Type() == cty.String {
							value = literal.Val.AsString()
						}
					}
				}
			}

			// Store source and version attributes
			switch keyName {
			case "source":
				source = value
			case "version":
				version = value
			}
		}

		// If we have both source and version, create the constraint mapping
		if source != "" && version != "" {
			// Normalize the source address to full registry format
			providerAddr := normalizeProviderAddress(opts, source)
			constraints[providerAddr] = normalizeVersionConstraint(version)
		} else if source == "" && version != "" {
			// If only version is specified, assume it's a hashicorp provider
			registryDomain := tf.GetDefaultRegistryDomain(opts)
			providerAddr := fmt.Sprintf("%s/hashicorp/%s", registryDomain, name)
			constraints[providerAddr] = normalizeVersionConstraint(version)
		}
	}

	return constraints
}

// normalizeProviderAddress converts provider source to full registry format
func normalizeProviderAddress(opts *options.TerragruntOptions, source string) string {
	parts := strings.Split(source, "/")
	registryDomain := tf.GetDefaultRegistryDomain(opts)

	const (
		singlePart    = 1
		twoPartPath   = 2
		threePartPath = 3
	)

	switch len(parts) {
	case singlePart:
		// "aws" -> "registry.terraform.io/hashicorp/aws" or "registry.opentofu.org/hashicorp/aws"
		return fmt.Sprintf("%s/hashicorp/%s", registryDomain, parts[0])
	case twoPartPath:
		// "hashicorp/aws" -> "registry.terraform.io/hashicorp/aws" or "registry.opentofu.org/hashicorp/aws"
		return fmt.Sprintf("%s/%s", registryDomain, source)
	case threePartPath:
		// "registry.terraform.io/hashicorp/aws" -> keep as is
		return source
	default:
		// Fallback to original if format is unexpected
		return source
	}
}

// normalizeVersionConstraint normalizes version constraints to the format expected by OpenTofu/Terraform lockfiles.
//
// This includes:
// 1. Removing the "=" prefix if present
// 2. Normalizing version numbers to full 3-part format (e.g., "2.2" becomes "2.2.0")
func normalizeVersionConstraint(constraint string) string {
	constraint = strings.TrimSpace(constraint)

	// If constraint starts with "=", remove it
	if after, ok := strings.CutPrefix(constraint, "="); ok {
		constraint = strings.TrimSpace(after)
	}

	// Split the constraint into operator and version parts
	parts := strings.Fields(constraint)

	// This is just a version number without an operator (e.g., "1.2.3")
	const justVersionParts = 1
	if len(parts) == justVersionParts {
		if v, err := version.NewVersion(parts[0]); err == nil {
			return v.String()
		}

		return constraint
	}

	// This has an operator and version (e.g., ">= 2.2")
	const operatorAndVersionParts = 2
	if len(parts) == operatorAndVersionParts {
		operator := parts[0]
		versionStr := parts[1]

		// Parse the version to normalize it
		if v, err := version.NewVersion(versionStr); err == nil {
			// The version library normalizes "2.2" to "2.2.0"
			return fmt.Sprintf("%s %s", operator, v.String())
		}
	}

	// If parsing fails or unexpected format, return the original constraint
	return constraint
}
