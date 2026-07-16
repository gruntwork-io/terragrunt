package getter

import "os"

// MaxDecompressedFiles exposes the shipped entry bound so tests exercise the
// value production actually enforces.
const MaxDecompressedFiles = ociMaxDecompressedFiles

// ExtractModuleWithLimits exposes bounded extraction to the external test
// package, so the byte bound can be tripped without staging a
// production-sized archive.
func (g *OCIGetter) ExtractModuleWithLimits(
	zipPath, subDir, dstPath, source string,
	umask os.FileMode,
	sizeLimit int64,
	filesLimit int,
) error {
	return g.extractModuleWithLimits(zipPath, subDir, dstPath, source, umask, sizeLimit, filesLimit)
}
