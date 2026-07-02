package git

import (
	"bytes"
	"context"
	"errors"
	"strings"
)

// SubmoduleURLs returns the submodule path → URL table declared by the
// .gitmodules blob at gitmodulesHash, read with `git config --blob` so
// the parsing matches git's own config syntax. Entries missing either
// a path or a url are dropped. The blob must exist in the repository
// at [GitRunner.WorkDir].
func (g *GitRunner) SubmoduleURLs(ctx context.Context, gitmodulesHash string) (map[string]string, error) {
	if err := g.RequiresWorkDir(); err != nil {
		return nil, err
	}

	cmd := g.prepareCommand(ctx, "config", "--blob", gitmodulesHash, "--list", "-z")

	var stdout, stderr bytes.Buffer

	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		return nil, &WrappedError{
			Op:      "git_config_blob",
			Context: stderr.String(),
			Err:     errors.Join(ErrCommandSpawn, err),
		}
	}

	return ParseSubmoduleConfig(stdout.String()), nil
}

// ParseSubmoduleConfig parses `git config --list -z` output into a
// submodule path → URL table. The -z form terminates each record with
// NUL and separates the key from the value with a newline, so values
// containing newlines survive. Records other than submodule.<name>.path
// and submodule.<name>.url are ignored, and submodule names keep any
// embedded dots because only the fixed prefix and suffix are stripped.
func ParseSubmoduleConfig(output string) map[string]string {
	paths := map[string]string{}
	urls := map[string]string{}

	for record := range strings.SplitSeq(output, "\x00") {
		key, value, ok := strings.Cut(record, "\n")
		if !ok {
			continue
		}

		name, ok := strings.CutPrefix(key, "submodule.")
		if !ok {
			continue
		}

		if sub, ok := strings.CutSuffix(name, ".path"); ok {
			paths[sub] = value
			continue
		}

		if sub, ok := strings.CutSuffix(name, ".url"); ok {
			urls[sub] = value
		}
	}

	resolved := make(map[string]string, len(paths))

	for name, path := range paths {
		if url, ok := urls[name]; ok {
			resolved[path] = url
		}
	}

	return resolved
}

// ResolveSubmoduleURL resolves a .gitmodules url value against the
// remote URL of the repository declaring the submodule, following
// git's relative-url rules: each leading "../" removes one trailing
// path component from parentURL, "./" segments are dropped, and
// anything that does not start with one of those prefixes is returned
// unchanged. When an SCP-form parent (git@host:path) runs out of path
// components, the last one is chopped at the colon and the colon is
// restored on the final join, so "git@host:a/b.git" resolved against
// "../../c.git" yields "git@host:c.git".
func ResolveSubmoduleURL(parentURL, url string) string {
	if !strings.HasPrefix(url, "./") && !strings.HasPrefix(url, "../") {
		return url
	}

	base := strings.TrimSuffix(parentURL, "/")
	colonsep := false

	for {
		if rest, ok := strings.CutPrefix(url, "./"); ok {
			url = rest
			continue
		}

		rest, ok := strings.CutPrefix(url, "../")
		if !ok {
			break
		}

		url = rest

		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[:i]
			continue
		}

		if i := strings.LastIndexByte(base, ':'); i >= 0 {
			base = base[:i]
			colonsep = true

			continue
		}

		// Out of components. Git dies here for local parents and
		// degrades for remote ones; "." keeps the join well-formed
		// without inventing a path.
		base = "."
	}

	if colonsep {
		return base + ":" + url
	}

	return base + "/" + url
}
