package getproviders

import (
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// ProviderConstraints maps provider addresses to their version constraints from required_providers blocks
type ProviderConstraints map[string]string

// ParseProviderConstraints parses all .tf and .tofu files in the given directory and extracts required_providers constraints
func ParseProviderConstraints(
	fsys vfs.FS,
	env map[string]string,
	impl tfimpl.Type,
	workingDir string,
) (ProviderConstraints, error) {
	// Whether env is consulted at all depends on what the directory holds, so
	// asserting at the first read would let a nil pass unnoticed until some
	// unrelated unit happened to declare a provider.
	venv.RequireEnvMap(env)

	constraints := make(ProviderConstraints)

	entries, err := vfs.ReadDir(fsys, workingDir)
	if err != nil {
		// A unit whose directory has not been materialized yet constrains
		// nothing, which is the same answer an empty directory gives.
		if errors.Is(err, fs.ErrNotExist) {
			return constraints, nil
		}

		return nil, err
	}

	// A module directory holds mostly `.tf` files and rarely a `.tofu` file, so
	// `tfFiles` is the only slice worth sizing up front.
	tfFiles := make([]string, 0, len(entries))

	var tofuFiles []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		switch filepath.Ext(entry.Name()) {
		case ".tf":
			tfFiles = append(tfFiles, filepath.Join(workingDir, entry.Name()))
		case ".tofu":
			tofuFiles = append(tofuFiles, filepath.Join(workingDir, entry.Name()))
		}
	}

	// A provider declared in both a .tf and a .tofu file takes the .tofu
	// constraint, so the .tofu files are merged last.
	for _, file := range slices.Concat(tfFiles, tofuFiles) {
		fileConstraints, err := parseProviderConstraintsFromFile(fsys, env, impl, file)
		if err != nil {
			// One file that does not parse must not cost the constraints the
			// rest of the directory declares.
			continue
		}

		maps.Copy(constraints, fileConstraints)
	}

	return constraints, nil
}

// parseProviderConstraintsFromFile parses a single .tf file and extracts required_providers constraints
func parseProviderConstraintsFromFile(
	fsys vfs.FS,
	env map[string]string,
	impl tfimpl.Type,
	filename string,
) (ProviderConstraints, error) {
	constraints := make(ProviderConstraints)

	content, err := vfs.ReadFile(fsys, filename)
	if err != nil {
		return nil, err
	}

	// Parse the HCL file
	file, diags := hclsyntax.ParseConfig(content, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, diags
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
			providerConstraints := parseProvidersFromRequiredProvidersBlock(env, impl, nestedBlock)

			// Merge constraints from this required_providers block
			maps.Copy(constraints, providerConstraints)
		}
	}

	return constraints, nil
}

// parseProvidersFromRequiredProvidersBlock extracts provider constraints from a required_providers block
func parseProvidersFromRequiredProvidersBlock(
	env map[string]string,
	impl tfimpl.Type,
	block *hclsyntax.Block,
) ProviderConstraints {
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
			providerAddr := normalizeProviderAddress(env, impl, source)
			constraints[providerAddr] = normalizeVersionConstraint(version)
		} else if source == "" && version != "" {
			// If only version is specified, assume it's a hashicorp provider
			registryDomain := tfimpl.DefaultRegistryDomain(env, impl)
			providerAddr := fmt.Sprintf("%s/hashicorp/%s", registryDomain, name)
			constraints[providerAddr] = normalizeVersionConstraint(version)
		}
	}

	return constraints
}

// normalizeProviderAddress converts provider source to full registry format
func normalizeProviderAddress(env map[string]string, impl tfimpl.Type, source string) string {
	parts := strings.Split(source, "/")
	registryDomain := tfimpl.DefaultRegistryDomain(env, impl)

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
// 3. Handling multi-part constraints (e.g., ">= 3.0, < 7.0" becomes ">= 3.0.0, < 7.0.0")
func normalizeVersionConstraint(constraint string) string {
	constraint = strings.TrimSpace(constraint)

	parts := strings.Split(constraint, ",")
	normalized := make([]string, 0, len(parts))

	for _, part := range parts {
		normalized = append(normalized, normalizeSingleConstraint(strings.TrimSpace(part)))
	}

	return strings.Join(normalized, ", ")
}

// normalizeSingleConstraint normalizes a single version constraint (no commas).
func normalizeSingleConstraint(constraint string) string {
	if after, ok := strings.CutPrefix(constraint, "="); ok {
		constraint = strings.TrimSpace(after)
	}

	fields := strings.Fields(constraint)

	const justVersionParts = 1
	if len(fields) == justVersionParts {
		if v, err := version.NewVersion(fields[0]); err == nil {
			return v.String()
		}

		return constraint
	}

	const operatorAndVersionParts = 2
	if len(fields) == operatorAndVersionParts {
		operator := fields[0]
		versionStr := fields[1]

		if v, err := version.NewVersion(versionStr); err == nil {
			return fmt.Sprintf("%s %s", operator, v.String())
		}
	}

	return constraint
}
