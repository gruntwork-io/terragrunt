package engine

import (
	"context"
	"fmt"
	"github.com/gruntwork-io/terragrunt/options"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/v55/github"
)

func EnginePath(engineOptions options.EngineOptions) (string, error) {

	return "", nil
}

func DownloadEngine(engineOptions options.EngineOptions) error {

	return nil
}

// fetchRelease gets the specified GitHub release
func fetchRelease(ctx context.Context, client *github.Client, config EngineOptions) (*github.RepositoryRelease, error) {
	owner, repo := splitRepoSource(config.Source)
	if config.Version == "latest" {
		return client.Repositories.GetLatestRelease(ctx, owner, repo)
	} else {
		return client.Repositories.GetReleaseByTag(ctx, owner, repo, config.Version)
	}
}

// isValidAsset checks if an asset name matches the Terragrunt engine naming pattern
func isValidAsset(name string, config EngineOptions) bool {
	pattern := fmt.Sprintf(`terragrunt-%s-%s_\d+\.\d+\.\d+_[a-z0-9]+_[a-z0-9]+\.zip`,
		regexp.QuoteMeta(config.Type), regexp.QuoteMeta(strings.ReplaceAll(config.Source, "/", "-")))
	return regexp.MustCompile(pattern).MatchString(name)
}

// splitRepoSource splits the "owner/repo" string from the config
func splitRepoSource(source string) (string, string) {
	parts := strings.Split(source, "/")
	return parts[0], parts[1]
}

// downloadAsset downloads and saves a GitHub release asset
func downloadAsset(ctx context.Context, client *github.Client, url, outPath string) error {
	req, _ := client.NewRequest("GET", url, nil)
	resp, err := client.Do(ctx, req, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

// buildOutputPath constructs the full output path for the asset
func buildOutputPath(baseDir, assetName string, config EngineOptions) string {
	parts := strings.Split(assetName, "_")
	pluginName := strings.Split(config.Source, "/")[1]

	return filepath.Join(
		baseDir, pluginName, config.Type, config.Version,
		parts[3], parts[4], assetName,
	)
}
