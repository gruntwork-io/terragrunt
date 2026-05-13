package shell

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-version"
)

const (
	gitPrefix = "git::"
	refsTags  = "refs/tags/"

	tagSplitPart = 2

	// maxNestedGitScanDepth caps the nested-repo guard's walk so a path
	// whose Dir() never reaches a fixed point cannot hang the process.
	maxNestedGitScanDepth = 1024
)

// NestedGitScanDepthExceededError is returned when the nested-repo guard's
// walk exceeds maxNestedGitScanDepth. Surfacing this as a typed error keeps
// callers from mistaking an aborted scan for a clean one.
type NestedGitScanDepthExceededError struct {
	Path     string
	Root     string
	MaxDepth int
}

func (e *NestedGitScanDepthExceededError) Error() string {
	return fmt.Sprintf("nested-git scan exceeded depth %d walking from %q to %q", e.MaxDepth, e.Path, e.Root)
}

// GitTopLevelDir returns the git repository root that contains path,
// memoized in a run-scoped cache. A `.git` scan between path and any cached
// ancestor keeps the answer correct when a nested repository sits below an
// already-cached outer root. Concurrent misses for the same repo collapse to
// a single fork via the cache's resolve lock and a re-check after acquiring it.
//
// The git invocation runs with v.Exec and v.Env; its stdout and stderr are
// captured to local buffers, so v.Writers is overridden for this call.
func GitTopLevelDir(ctx context.Context, l log.Logger, v venv.Venv, path string) (string, error) {
	repoRoots := cache.ContextRepoRootCache(ctx, cache.RepoRootCacheContextKey)
	normalized := normalizeRepoPath(path)

	if root, ok, err := lookupRepoRoot(ctx, repoRoots, normalized); err != nil {
		return "", err
	} else if ok {
		return root, nil
	}

	repoRoots.BeginResolve()
	defer repoRoots.EndResolve()

	if root, ok, err := lookupRepoRoot(ctx, repoRoots, normalized); err != nil {
		return "", err
	} else if ok {
		return root, nil
	}

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	gitV := v
	gitV.Writers = writer.Writers{Writer: &stdout, ErrWriter: &stderr}

	gitRunOpts := NewShellOptions().WithWorkingDir(path)

	cmd, err := RunCommandWithOutput(ctx, l, gitV, gitRunOpts, path, true, false, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}

	// Git on Windows always emits forward slashes from `rev-parse --show-toplevel`,
	// so normalize to OS-native separators to stay consistent with the other path
	// HCL functions (get_terragrunt_dir, find_in_parent_folders, etc.).
	cmdOutput := filepath.FromSlash(strings.TrimSpace(cmd.Stdout.String()))

	if stderrString := strings.TrimSpace(stderr.String()); stderrString != "" {
		l.Warnf("git rev-parse --show-toplevel resulted in stderr output: \n%v\n", stderrString)
	}

	l.Debugf("git show-toplevel result: %s", cmdOutput)

	repoRoots.Add(ctx, normalizeRepoPath(cmdOutput))

	return cmdOutput, nil
}

// lookupRepoRoot returns the cached root for path when the nested-repo guard
// accepts it. A nested `.git` finding is reported as a miss (false, nil) so
// the caller falls through to a fresh git resolution.
func lookupRepoRoot(ctx context.Context, repoRoots *cache.RepoRootCache, path string) (string, bool, error) {
	cached, ok := repoRoots.Lookup(ctx, path)
	if !ok {
		return "", false, nil
	}

	nested, err := hasNestedGit(path, cached)
	if err != nil {
		return "", false, err
	}

	if nested {
		return "", false, nil
	}

	return cached, true, nil
}

// hasNestedGit reports whether any directory between path and root (exclusive
// of root) contains a `.git` entry, meaning a nested repository sits below
// root and would invalidate root as the answer for path.
func hasNestedGit(path, root string) (bool, error) {
	if path == root {
		return false, nil
	}

	current := path
	for range maxNestedGitScanDepth {
		if current == root {
			return false, nil
		}

		_, err := os.Stat(filepath.Join(current, ".git"))
		if err == nil {
			return true, nil
		}

		if !os.IsNotExist(err) {
			return false, err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return false, nil
		}

		current = parent
	}

	return false, &NestedGitScanDepthExceededError{
		Path:     path,
		Root:     root,
		MaxDepth: maxNestedGitScanDepth,
	}
}

// normalizeRepoPath canonicalizes a directory path for cache comparison. The
// EvalSymlinks step is best-effort: failures (e.g. the path does not exist)
// fall through to the lexical clean so the surrounding git call still
// produces the real error.
func normalizeRepoPath(path string) string {
	if path == "" {
		return ""
	}

	cleaned := filepath.Clean(path)

	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return resolved
	}

	return cleaned
}

// GitRepoTags fetches git repository tags from passed url.
func GitRepoTags(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	workingDir string,
	gitRepo *url.URL,
) ([]string, error) {
	repoPath := gitRepo.String()
	// remove git:: part if present
	repoPath = strings.TrimPrefix(repoPath, gitPrefix)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	gitV := v
	gitV.Writers = writer.Writers{Writer: &stdout, ErrWriter: &stderr}

	gitRunOpts := NewShellOptions().WithWorkingDir(workingDir)

	output, err := RunCommandWithOutput(ctx, l, gitV, gitRunOpts, workingDir, true, false, "git", "ls-remote", "--tags", repoPath)
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
func GitLastReleaseTag(ctx context.Context, l log.Logger, v venv.Venv, workingDir string, gitRepo *url.URL) (string, error) {
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
