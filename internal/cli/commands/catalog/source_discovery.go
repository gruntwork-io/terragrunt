package catalog

import (
	"context"
	"strings"

	"github.com/hashicorp/go-getter"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// DiscoverSourceURLs walks the project tree starting from pctx.RootWorkingDir,
// finds all terragrunt.hcl files, parses them to extract terraform.source URLs,
// normalizes them to repo-level URLs, and returns a deduplicated list.
func DiscoverSourceURLs(ctx context.Context, l log.Logger, pctx *config.ParsingContext) ([]string, error) {
	configFiles, err := config.FindConfigFilesInPath(pctx.RootWorkingDir, pctx.Experiments, config.DefaultTerragruntConfigPath, nil, "")
	if err != nil {
		return nil, err
	}

	sourcePctx := pctx.WithDecodeList(config.TerraformSource).WithDiagnosticsSuppressed(l)

	var repoURLs []string

	for _, configFile := range configFiles {
		fileLogger, filePctx, err := sourcePctx.WithConfigPath(l, configFile)
		if err != nil {
			l.Debugf("Skipping %s: %v", configFile, err)
			continue
		}

		cfg, err := config.PartialParseConfigFile(ctx, filePctx, fileLogger, configFile, nil)
		if err != nil {
			l.Debugf("Skipping %s: failed to parse: %v", configFile, err)
			continue
		}

		if cfg.Terraform == nil || cfg.Terraform.Source == nil {
			continue
		}

		repoURL := ExtractRepoURL(*cfg.Terraform.Source)
		if repoURL == "" {
			continue
		}

		repoURLs = append(repoURLs, repoURL)
	}

	return util.RemoveDuplicates(repoURLs), nil
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
