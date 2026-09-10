package shell

import (
	"bytes"
	"context"
	"net/url"
	"strings"

	"github.com/hashicorp/go-version"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	gitPrefix = "git::"
	refsTags  = "refs/tags/"

	tagSplitPart = 2
)

// GitRepoTags fetches git repository tags from passed url.
func GitRepoTags(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	workingDir string,
	gitRepo *url.URL,
) ([]string, error) {
	repoPath := gitRepo.String()
	// remove git:: part if present
	repoPath = strings.TrimPrefix(repoPath, gitPrefix)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	gitV := v.WithWriter(&stdout).WithErrWriter(&stderr)

	gitRunOpts := NewShellOptions(v.Env).WithWorkingDir(workingDir)

	output, err := RunCommandWithOutput(
		ctx,
		l,
		gitV,
		gitRunOpts,
		workingDir,
		true,
		false,
		"git",
		"ls-remote",
		"--tags",
		repoPath,
	)
	if err != nil {
		return nil, err
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
func GitLastReleaseTag(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	workingDir string,
	gitRepo *url.URL,
) (string, error) {
	tags, err := GitRepoTags(ctx, l, v, workingDir, gitRepo)
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
