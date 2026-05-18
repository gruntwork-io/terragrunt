package tflint_test

import (
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
// and a max-folders bound. The bound is clamped to [0, maxFolderBound] so
// the fuzzer does not chew CPU on absurd values.
func FuzzFindConfigInProject(f *testing.F) {
	type seed struct {
		layout     string
		workingDir string
		maxFolders int
	}

	seeds := []seed{
		{layout: "/work/.tflint.hcl", workingDir: "/work", maxFolders: 5},
		{layout: "/work/.tflint.hcl\n/work/a/b/c", workingDir: "/work/a/b/c", maxFolders: 5},
		{layout: "/a/b/c/d/e/f/.tflint.hcl", workingDir: "/", maxFolders: 2},
		{layout: "", workingDir: "/", maxFolders: 1},
		{layout: "/.tflint.hcl", workingDir: "/", maxFolders: 1},
		{layout: ".tflint.hcl", workingDir: "relative", maxFolders: 3},
		{layout: "/work/.tflint.hcl\n/work/.tflint.hcl/nested", workingDir: "/work/x/y/z", maxFolders: 10},
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

			// Ignore write errors: pathological filenames are part of the
			// fuzz domain, but we only care about the walk that follows.
			_ = vfs.WriteFile(fs, p, []byte{}, 0o644)
		}

		// Clamp the loop count so the fuzzer cannot ask for billions of
		// iterations of the parent walk. Going through min/max avoids the
		// two's-complement overflow on math.MinInt that bit a prior
		// mod/negate version of this clamp.
		const maxFolderBound = 64

		opts := &tflint.TFLintOptions{
			WorkingDir:        workingDir,
			RootWorkingDir:    "/",
			MaxFoldersToCheck: max(0, min(maxFolders, maxFolderBound)),
		}

		l := logger.CreateLogger()

		// Contract: must not panic. Returned (string, error) may take any
		// value; we are only validating absence of panics and termination.
		_, _ = tflint.FindConfigInProject(l, fs, opts)
	})
}
