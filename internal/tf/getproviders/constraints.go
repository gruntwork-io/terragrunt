package getproviders

import (
	"cmp"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"errors"

	semver "github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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

	for name, attr := range block.Body.Attributes {
		source, version := parseProviderRequirement(attr.Expr)
		if version == "" {
			continue
		}

		// OpenTofu and Terraform imply the hashicorp namespace for an entry with no source.
		if source == "" {
			source = name
		}

		constraints[normalizeProviderAddress(env, impl, source)] = normalizeVersionConstraint(version)
	}

	return constraints
}

// parseProviderRequirement extracts the source and version from one entry of a
// required_providers block. An entry is usually an object with a source and a
// version. OpenTofu and Terraform also accept a bare version constraint, the
// shorthand from before provider source addresses existed.
func parseProviderRequirement(expr hclsyntax.Expression) (string, string) {
	objExpr, ok := expr.(*hclsyntax.ObjectConsExpr)
	if !ok {
		return "", stringLiteral(expr)
	}

	var source, version string

	for _, item := range objExpr.Items {
		value := stringLiteral(item.ValueExpr)

		switch objectKeyName(item.KeyExpr) {
		case "source":
			source = value
		case "version":
			version = value
		}
	}

	return source, version
}

// objectKeyName returns the name of an object key written as an unquoted
// identifier or a plain string. Any other key yields the empty string.
func objectKeyName(expr hclsyntax.Expression) string {
	keyExpr, ok := expr.(*hclsyntax.ObjectConsKeyExpr)
	if !ok {
		return ""
	}

	switch wrapped := keyExpr.Wrapped.(type) {
	case *hclsyntax.TemplateExpr:
		return stringLiteral(wrapped)
	case *hclsyntax.ScopeTraversalExpr:
		if len(wrapped.Traversal) == 1 {
			if root, ok := wrapped.Traversal[0].(hcl.TraverseRoot); ok {
				return root.Name
			}
		}
	case *hclsyntax.LiteralValueExpr:
		if wrapped.Val.Type() == cty.String {
			return wrapped.Val.AsString()
		}
	}

	return ""
}

// stringLiteral returns the value of an expression that is a plain string literal.
// An expression that needs evaluating, such as an interpolation, yields the empty string.
func stringLiteral(expr hclsyntax.Expression) string {
	templateExpr, ok := expr.(*hclsyntax.TemplateExpr)
	if !ok {
		return ""
	}

	if len(templateExpr.Parts) != 1 {
		return ""
	}

	literal, ok := templateExpr.Parts[0].(*hclsyntax.LiteralValueExpr)
	if !ok {
		return ""
	}

	if literal.Val.Type() != cty.String {
		return ""
	}

	return literal.Val.AsString()
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
// 3. Writing a pessimistic constraint with two parts unless it declares three (e.g., "~> 3.0" stays "~> 3.0")
// 4. Handling multi-part constraints (e.g., ">= 3.0, < 7.0" becomes ">= 3.0.0, < 7.0.0")
// 5. Sorting the terms by version and dropping repeats (e.g., "< 7.0, >= 3.0, >= 3.0.0" becomes ">= 3.0.0, < 7.0.0")
//
// A constraint with a term whose operator or version is not recognized keeps
// its declared order and repeats.
func normalizeVersionConstraint(constraint string) string {
	parts := strings.Split(strings.TrimSpace(constraint), ",")
	terms := make([]constraintTerm, 0, len(parts))
	recognized := true

	for _, part := range parts {
		term, ok := normalizeSingleConstraint(strings.TrimSpace(part))
		recognized = recognized && ok

		terms = append(terms, term)
	}

	if recognized {
		slices.SortFunc(terms, compareConstraintTerms)

		terms = slices.CompactFunc(terms, func(a, b constraintTerm) bool {
			return a.text == b.text
		})
	}

	normalized := make([]string, 0, len(terms))
	for _, term := range terms {
		normalized = append(normalized, term.text)
	}

	return strings.Join(normalized, ", ")
}

// constraintTerm is one comma-separated term of a version constraint, written
// the way tofu writes it to a lock file.
type constraintTerm struct {
	version *semver.Version
	text    string
	rank    operatorRank
}

// operatorRank orders terms that share a version. The order is the one tofu
// uses when it writes lock file constraints.
type operatorRank int

const (
	rankGreaterThan operatorRank = iota + 1
	rankGreaterThanOrEqual
	rankEqual
	rankPessimisticPatch
	rankPessimisticMinor
	rankLessThanOrEqual
	rankLessThan
	rankNotEqual
)

// compareConstraintTerms orders terms by version, then by build metadata, then
// by operator.
func compareConstraintTerms(a, b constraintTerm) int {
	return cmp.Or(
		a.version.Compare(b.version),
		strings.Compare(a.version.Metadata(), b.version.Metadata()),
		cmp.Compare(a.rank, b.rank),
	)
}

// normalizeSingleConstraint normalizes a single version constraint (no commas).
// It reports false, with the term as declared, when the operator or version is
// not recognized.
func normalizeSingleConstraint(constraint string) (constraintTerm, bool) {
	const operatorChars = "=<>!~"

	rest := strings.TrimLeft(constraint, operatorChars)
	operator := constraint[:len(constraint)-len(rest)]
	declared := strings.TrimSpace(rest)

	v, err := semver.Parse(declared)
	if err != nil {
		return constraintTerm{text: constraint}, false
	}

	term := constraintTerm{version: v, text: operator + " " + v.String()}

	switch operator {
	case "", "=":
		term.text = v.String()
		term.rank = rankEqual
	case "~>":
		term.rank = rankPessimisticPatch

		if declaresMinorOnly(declared) {
			term.text = operator + " " + minorVersion(v)
			term.rank = rankPessimisticMinor
		}
	case ">":
		term.rank = rankGreaterThan
	case ">=":
		term.rank = rankGreaterThanOrEqual
	case "<=":
		term.rank = rankLessThanOrEqual
	case "<":
		term.rank = rankLessThan
	case "!=":
		term.rank = rankNotEqual
	default:
		return constraintTerm{text: constraint}, false
	}

	return term, true
}

// declaresMinorOnly reports whether a `~>` version is declared with one or two
// parts. The number of parts sets which part may vary, so `3.0` allows any 3.x
// release and `3.0.0` allows only 3.0.x.
func declaresMinorOnly(declared string) bool {
	const (
		minorOnlyParts = 2
		digitsAndDots  = "0123456789."
	)

	numeric := strings.TrimPrefix(declared, "v")
	numeric = numeric[:len(numeric)-len(strings.TrimLeft(numeric, digitsAndDots))]

	return strings.Count(numeric, ".")+1 <= minorOnlyParts
}

// minorVersion formats v with two parts, the way tofu writes a `~>` version
// declared with one or two.
func minorVersion(v *semver.Version) string {
	segments := v.Segments()
	version := fmt.Sprintf("%d.%d", segments[0], segments[1])

	if prerelease := v.Prerelease(); prerelease != "" {
		version += "-" + prerelease
	}

	if metadata := v.Metadata(); metadata != "" {
		version += "+" + metadata
	}

	return version
}
