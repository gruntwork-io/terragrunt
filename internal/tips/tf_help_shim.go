package tips

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// maxLikelyShimSize intentionally sits well below typical OpenTofu/Terraform
// binaries, which are tens of megabytes, while allowing small shell-based shims.
const maxLikelyShimSize = 10 * 1024 * 1024

// GiveTFHelpShimTip emits the OpenTofuTerraformShim tip when the configured
// OpenTofu/Terraform executable looks like a small text-based shim. This gives
// users a targeted hint only after Terragrunt failed to retrieve command help.
func GiveTFHelpShimTip(l log.Logger, fs vfs.FS, tfPath, workingDir, pathEnv string, allTips Tips) {
	if allTips == nil {
		return
	}

	tip := allTips.Find(OpenTofuTerraformShim)
	if tip == nil {
		return
	}

	binaryPath := resolveBinaryPath(fs, tfPath, workingDir, pathEnv)
	if binaryPath == "" || !isLikelyTextShim(fs, binaryPath) {
		return
	}

	tip.Evaluate(l)
}

func resolveBinaryPath(fs vfs.FS, tfPath, workingDir, pathEnv string) string {
	if filepath.IsAbs(tfPath) {
		if exists, _ := vfs.FileExists(fs, tfPath); exists {
			return tfPath
		}

		return ""
	}

	if strings.Contains(tfPath, string(filepath.Separator)) {
		candidate := filepath.Join(workingDir, tfPath)
		if exists, _ := vfs.FileExists(fs, candidate); exists {
			return candidate
		}

		return ""
	}

	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = workingDir
		}

		candidate := filepath.Join(dir, tfPath)
		info, err := fs.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate
		}
	}

	return ""
}

func isLikelyTextShim(fs vfs.FS, path string) bool {
	info, err := fs.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > maxLikelyShimSize {
		return false
	}

	file, err := fs.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, 512))
	if err != nil {
		return false
	}

	return !bytes.ContainsRune(contents, '\x00') && utf8.Valid(contents)
}
