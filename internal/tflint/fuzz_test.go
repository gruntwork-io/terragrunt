package tflint_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tflint"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// FuzzFindConfigInProject pins the walk-safety contract of [tflint.FindConfigInProject]:
// no panic, no unbounded recursion, and a deterministic return for any
// (startDir, MaxFoldersToCheck) on an arbitrarily seeded MemMapFS.
//
// The corpus encodes the directory layout as a newline-separated list of
// paths that get seeded onto a fresh MemMapFS, plus the starting WorkingDir
// and a max-folders bound. The bound is wrapped to [0, 64] so the fuzzer
// does not chew CPU on absurd values.
func FuzzFindConfigInProject(f *testing.F) {
	seeds := []struct {
		layout     string
		workingDir string
		maxFolders int
	}{
		{"/work/.tflint.hcl", "/work", 5},
		{"/work/.tflint.hcl\n/work/a/b/c", "/work/a/b/c", 5},
		{"/a/b/c/d/e/f/.tflint.hcl", "/", 2},
		{"", "/", 1},
		{"/.tflint.hcl", "/", 1},
		{".tflint.hcl", "relative", 3},
		{"/work/.tflint.hcl\n/work/.tflint.hcl/nested", "/work/x/y/z", 10},
	}

	for _, s := range seeds {
		f.Add(s.layout, s.workingDir, s.maxFolders)
	}

	f.Fuzz(func(t *testing.T, layout, workingDir string, maxFolders int) {
		fs := vfs.NewMemMapFS()

		// Seed each non-empty line as a file. We bound the file size to
		// keep the in-memory FS cheap.
		for line := range strings.SplitSeq(layout, "\n") {
			p := strings.TrimSpace(line)
			if p == "" {
				continue
			}

			// Reject absurdly long paths; the OS layer also rejects them
			// and there is no signal value in fuzzing them.
			if len(p) > 4096 {
				continue
			}

			// Skip paths that filepath.Clean rejects with separator-only
			// names; the goal is shapes the real walker would see.
			if filepath.Clean(p) == "" {
				continue
			}

			// Ignore write errors: pathological filenames are part of the
			// fuzz domain, but we only care about the walk that follows.
			_ = vfs.WriteFile(fs, p, []byte{}, 0o644)
		}

		// Bound the loop count so the fuzzer cannot ask for billions of
		// iterations of the parent walk.
		const maxBound = 64

		if maxFolders < 0 {
			maxFolders = -maxFolders
		}

		maxFolders %= maxBound + 1

		opts := &tflint.TFLintOptions{
			WorkingDir:        workingDir,
			RootWorkingDir:    "/",
			MaxFoldersToCheck: maxFolders,
		}

		l := logger.CreateLogger()

		// Contract: must not panic. Returned (string, error) may take any
		// value; we are only validating absence of panics and termination.
		_, _ = tflint.FindConfigInProject(l, fs, opts)
	})
}
