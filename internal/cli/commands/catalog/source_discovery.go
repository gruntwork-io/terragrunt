package catalog

import (
	"regexp"
	"strings"

	"github.com/hashicorp/go-getter"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

// terraformSourceReg matches a terraform block's source attribute with a literal string value.
// It intentionally excludes interpolated sources (containing $) so that only
// statically-known URLs are captured.
var terraformSourceReg = regexp.MustCompile(`(?s)terraform\s*\{[^}]*?source\s*=\s*"([^"$]+)"`)

// DiscoverSourceURLs walks the project tree starting from rootDir, finds all
// terragrunt.hcl files, extracts terraform.source URLs, normalizes them to
// repo-level URLs, and returns a deduplicated list.
func DiscoverSourceURLs(rootDir string, experiments experiment.Experiments) ([]string, error) {
	configFiles, err := config.FindConfigFilesInPath(rootDir, experiments, config.DefaultTerragruntConfigPath, nil, "")
	if err != nil {
		return nil, err
	}

	var repoURLs []string

	for _, configFile := range configFiles {
		content, err := util.ReadFileAsString(configFile)
		if err != nil {
			continue
		}

		source := ExtractTerraformSource(content)
		if source == "" {
			continue
		}

		repoURL := ExtractRepoURL(source)
		if repoURL == "" {
			continue
		}

		repoURLs = append(repoURLs, repoURL)
	}

	return util.RemoveDuplicates(repoURLs), nil
}

// ExtractTerraformSource extracts the terraform.source attribute value from
// HCL content. Returns an empty string if no literal source is found or if
// the source uses interpolation.
func ExtractTerraformSource(content string) string {
	matches := terraformSourceReg.FindStringSubmatch(content)
	if len(matches) < 2 { //nolint:mnd
		return ""
	}

	return matches[1]
}

// ExtractRepoURL normalizes a terraform source URL to a repo-level URL by
// stripping subdirectory paths, query parameters, and getter prefixes.
// Returns an empty string for local paths and registry sources.
func ExtractRepoURL(source string) string {
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") {
		return ""
	}

	if strings.HasPrefix(source, "tfr:") {
		return ""
	}

	// Split at // to separate repo URL from module subdirectory
	moduleURL, _ := getter.SourceDirSubdir(source)
	if moduleURL == "" {
		return ""
	}

	// Strip query parameters (e.g., ?ref=v1.0.0)
	if idx := strings.IndexByte(moduleURL, '?'); idx >= 0 {
		moduleURL = moduleURL[:idx]
	}

	// Strip getter-specific prefixes (e.g., git::, s3::, gcs::)
	if idx := strings.Index(moduleURL, "::"); idx >= 0 {
		moduleURL = moduleURL[idx+2:]
	}

	return moduleURL
}
