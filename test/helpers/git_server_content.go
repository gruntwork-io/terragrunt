package helpers

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Fixture references to the per-test git server take the form
// `git::__MIRROR_URL__//<subpath>` (or the SSH variant), and a module
// reached that way may in turn reference a sibling fixture with a
// relative `source = "../other"`. When a test asks the server to serve
// a subpath, the server must also serve everything that subpath
// references, transitively. These patterns extract those references.
var (
	mirrorAbsRefRe = regexp.MustCompile(`__MIRROR_(?:SSH_)?URL__//([^"?'\s]+)`)
	// mirrorRelRefRes match relative references that resolve within the
	// cloned repository: HCL/Terraform `source = "../x"` module sources
	// and boilerplate `template-url: ../x` dependencies. Both must be
	// followed so a cloned module's siblings are committed too.
	mirrorRelRefRes = []*regexp.Regexp{
		regexp.MustCompile(`source\s*=\s*"(\.\.?/[^"]*)"`),
		regexp.MustCompile(`template-url:\s*["']?(\.\.?/[^"'\s]+)`),
	}
)

// mirrorRefsInTree returns the repo-relative subpaths referenced by
// absolute `__MIRROR_URL__//` / `__MIRROR_SSH_URL__//` sources anywhere
// under dir. Unlike [mirrorRefsInSubtree] it ignores relative `../`
// sources: in a rendered fixture copy those resolve within the copy, not
// against the server. It is how [GitServer.RenderFixture] discovers the
// fixtures a rendered tree clones.
func mirrorRefsInTree(dir string) ([]string, error) {
	var refs []string

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		for _, m := range mirrorAbsRefRe.FindAllSubmatch(data, -1) {
			refs = append(refs, filepath.ToSlash(filepath.Clean(string(m[1]))))
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return refs, nil
}

// mirrorExpandClosure walks the reference graph outward from seeds
// (repo-relative fixture subpaths) and returns the transitively-closed
// set of directories that exist on disk. Seeds that don't resolve to a
// directory (e.g. download/non-existent-path, which exists only to
// provoke a clone failure) are followed no further and dropped from the
// result. Nested entries are removed so the content commit writes each
// file once.
func mirrorExpandClosure(fixturesDir string, seeds []string) ([]string, error) {
	repoRoot := filepath.Dir(filepath.Dir(fixturesDir))

	seen := make(map[string]bool, len(seeds))
	queue := append([]string(nil), seeds...)

	for len(queue) > 0 {
		dir := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		if seen[dir] {
			continue
		}

		seen[dir] = true

		if !isDir(filepath.Join(repoRoot, dir)) {
			continue
		}

		refs, err := mirrorRefsInSubtree(fixturesDir, filepath.Join(repoRoot, dir))
		if err != nil {
			return nil, err
		}

		queue = append(queue, refs...)
	}

	dirs := make([]string, 0, len(seen))

	for dir := range seen {
		if isDir(filepath.Join(repoRoot, dir)) {
			dirs = append(dirs, dir)
		}
	}

	return dropNestedDirs(dirs), nil
}

// mirrorRefsInSubtree returns the repo-relative fixture directories
// referenced by any file under dir (an absolute path), following both
// absolute `__MIRROR_URL__//` references and relative `../` module
// sources resolved against the referencing file's location.
func mirrorRefsInSubtree(fixturesDir, dir string) ([]string, error) {
	repoRoot := filepath.Dir(filepath.Dir(fixturesDir))

	var refs []string

	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		// Skip symlinks, matching walkFixturesRooted: WalkDir reports a
		// symlink as a non-dir entry, and reading one that points at a
		// directory fails. They never carry mirror references anyway.
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		// Refs are ASCII module sources, so matching on raw bytes finds
		// them in text fixtures and simply misses in any binary file.
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		for _, m := range mirrorAbsRefRe.FindAllSubmatch(data, -1) {
			refs = append(refs, filepath.ToSlash(filepath.Clean(string(m[1]))))
		}

		fileDir := filepath.Dir(path)

		for _, re := range mirrorRelRefRes {
			for _, m := range re.FindAllSubmatch(data, -1) {
				target := filepath.Join(fileDir, filepath.FromSlash(string(m[1])))

				rel, err := filepath.Rel(repoRoot, target)
				if err != nil {
					continue
				}

				refs = append(refs, filepath.ToSlash(rel))
			}
		}

		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return refs, nil
}

// dropNestedDirs removes any entry nested under another so the content
// commit writes each file exactly once. Sorting puts a parent ahead of
// everything beneath it.
func dropNestedDirs(dirs []string) []string {
	sort.Strings(dirs)

	out := make([]string, 0, len(dirs))

	for _, dir := range dirs {
		nested := false

		for _, kept := range out {
			if dir == kept || strings.HasPrefix(dir, kept+"/") {
				nested = true

				break
			}
		}

		if !nested {
			out = append(out, dir)
		}
	}

	return out
}

func isDir(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.IsDir()
}
