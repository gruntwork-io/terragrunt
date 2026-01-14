package shell

import (
	"bytes"
	"context"
	"net/url"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-version"
)

const (
	gitPrefix = "git::"
	refsTags  = "refs/tags/"

	tagSplitPart = 2
)

// GitTopLevelDir fetches git repository path from passed directory.
func GitTopLevelDir(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, path string) (string, error) {
	runCache := cache.ContextCache[string](ctx, cache.RunCmdCacheContextKey)
	cacheKey := "top-level-dir-" + path

	if gitTopLevelDir, found := runCache.Get(ctx, cacheKey); found {
		return gitTopLevelDir, nil
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	opts, err := options.NewTerragruntOptionsWithConfigPath(path)
	if err != nil {
		return "", err
	}

	opts.Env = terragruntOptions.Env
	opts.Writer = &stdout
	opts.ErrWriter = &stderr

	cmd, err := RunCommandWithOutput(ctx, l, opts, path, true, false, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}

	cmdOutput := strings.TrimSpace(cmd.Stdout.String())

	if stderrString := strings.TrimSpace(stderr.String()); stderrString != "" {
		l.Warnf("git rev-parse --show-toplevel resulted in stderr output: \n%v\n", stderrString)
	}

	l.Debugf("git show-toplevel result: %s", cmdOutput)

	runCache.Put(ctx, cacheKey, cmdOutput)

	return cmdOutput, nil
}

// GitRepoTags fetches git repository tags from passed url.
func GitRepoTags(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, gitRepo *url.URL) ([]string, error) {
	repoPath := gitRepo.String()
	// remove git:: part if present
	repoPath = strings.TrimPrefix(repoPath, gitPrefix)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	gitOpts, err := options.NewTerragruntOptionsWithConfigPath(opts.WorkingDir)
	if err != nil {
		return nil, err
	}

	gitOpts.Env = opts.Env
	gitOpts.Writer = &stdout
	gitOpts.ErrWriter = &stderr

	output, err := RunCommandWithOutput(ctx, l, opts, opts.WorkingDir, true, false, "git", "ls-remote", "--tags", repoPath)
	if err != nil {
		return nil, errors.New(err)
	}

	var tags []string

	tagLines := strings.SplitSeq(output.Stdout.String(), "\n")

	for line := range tagLines {
		fields := strings.Fields(line)
		if len(fields) >= tagSplitPart {
			tags = append(tags, fields[1])
		}
	}

	return tags, nil
}

// GitLastReleaseTag fetches git repository last release tag.
func GitLastReleaseTag(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, gitRepo *url.URL) (string, error) {
	tags, err := GitRepoTags(ctx, l, opts, gitRepo)
	if err != nil {
		return "", err
	}

	if len(tags) == 0 {
		return "", nil
	}

	return LastReleaseTag(tags), nil
}

// LastReleaseTag returns last release tag from passed tags slice.
func LastReleaseTag(tags []string) string {
	semverTags := extractSemVerTags(tags)
	if len(semverTags) == 0 {
		return ""
	}
	// find last semver tag
	lastVersion := semverTags[0]
	for _, ver := range semverTags {
		if ver.GreaterThanOrEqual(lastVersion) {
			lastVersion = ver
		}
	}

	return lastVersion.Original()
}

// extractSemVerTags - extract semver tags from passed tags slice.
func extractSemVerTags(tags []string) []*version.Version {
	var semverTags []*version.Version

	for _, tag := range tags {
		t := strings.TrimPrefix(tag, refsTags)
		if v, err := version.NewVersion(t); err == nil {
			// consider only semver tags
			semverTags = append(semverTags, v)
		}
	}

	return semverTags
}
