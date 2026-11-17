package tf

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

const (
	// InitFingerprintFile is the file in the data directory that records the [InitFingerprint]
	// taken right after the last successful init.
	InitFingerprintFile = "terragrunt-init-fingerprint"

	modulesManifestFile = "modules.json"
)

// InitFingerprint hashes what init reads and what it leaves behind for the root module at rootDir
// with the data directory dataDir: the configuration files of the root module and of every local
// module recorded in the modules manifest, the lock file, and whether the plugins, providers, and
// modules directories exist.
//
// Modules that init downloaded into the modules directory are left out, as nothing but init changes
// them. A recorded local module whose directory is gone hashes differently from one that is present.
func InitFingerprint(fsys vfs.FS, rootDir, dataDir string) (string, error) {
	const copyBufferSize = 32 * 1024

	fp := fingerprinter{
		h:       sha256.New(),
		fsys:    fsys,
		rootDir: rootDir,
		buf:     make([]byte, copyBufferSize),
	}

	if err := fp.hashConfigDir(rootDir); err != nil {
		return "", err
	}

	moduleDirs, err := localModuleDirs(fsys, rootDir, dataDir)
	if err != nil {
		return "", err
	}

	for _, dir := range moduleDirs {
		if !vfs.Exists(fsys, dir) {
			fp.writeEntry("missing", relSlash(rootDir, dir))

			continue
		}

		if err := fp.hashConfigDir(dir); err != nil {
			return "", err
		}
	}

	if err := fp.hashLockFile(); err != nil {
		return "", err
	}

	for _, name := range []string{"plugins", "providers", "modules"} {
		fp.writeEntry(name, strconv.FormatBool(vfs.Exists(fsys, filepath.Join(dataDir, name))))
	}

	return hex.EncodeToString(fp.h.Sum(nil)), nil
}

// WriteInitFingerprint records fingerprint in [InitFingerprintFile] under dataDir, creating the
// directory when init left none behind.
func WriteInitFingerprint(fsys vfs.FS, dataDir, fingerprint string) error {
	const dirPerms = 0o755

	if err := fsys.MkdirAll(dataDir, dirPerms); err != nil {
		return err
	}

	const filePerms = 0o644

	return vfs.WriteFile(fsys, filepath.Join(dataDir, InitFingerprintFile), []byte(fingerprint), filePerms)
}

// fingerprinter accumulates one [InitFingerprint], streaming every file through buf.
type fingerprinter struct {
	h       hash.Hash
	fsys    vfs.FS
	rootDir string
	buf     []byte
}

func (fp *fingerprinter) hashConfigDir(dir string) error {
	fp.writeEntry("dir", relSlash(fp.rootDir, dir))

	files, err := util.ListTfFiles(fp.fsys, dir)
	if err != nil {
		return err
	}

	for _, path := range files {
		sum, err := fp.fileSum(path)
		if err != nil {
			return err
		}

		fp.writeEntry("file", relSlash(fp.rootDir, path))
		fp.writeEntry("sha256", sum)
	}

	return nil
}

func (fp *fingerprinter) hashLockFile() error {
	path := filepath.Join(fp.rootDir, TerraformLockFile)
	if !vfs.Exists(fp.fsys, path) {
		fp.writeEntry("lock", "absent")

		return nil
	}

	sum, err := fp.fileSum(path)
	if err != nil {
		return err
	}

	fp.writeEntry("lock", sum)

	return nil
}

// fileSum streams the file at path through SHA-256 and returns the hex digest.
func (fp *fingerprinter) fileSum(path string) (_ string, retErr error) {
	f, err := fp.fsys.Open(path)
	if err != nil {
		return "", err
	}

	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	sum := sha256.New()

	// An *os.File implements io.WriterTo, which io.CopyBuffer prefers over buf and which
	// allocates its own buffer per call. Hiding it keeps every file on the one shared buffer.
	if _, err := io.CopyBuffer(sum, struct{ io.Reader }{f}, fp.buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(sum.Sum(nil)), nil
}

// writeEntry folds one labeled value into the fingerprint with a length prefix, so that two
// different sequences of entries never hash alike.
func (fp *fingerprinter) writeEntry(label, value string) {
	fp.h.Write(fmt.Appendf(nil, "%s:%d:%s\n", label, len(value), value))
}

// relSlash returns path relative to rootDir with forward slashes, or path itself when no
// relative form exists, so the fingerprint does not depend on where the root module lives.
func relSlash(rootDir, path string) string {
	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}

	return filepath.ToSlash(rel)
}

type modulesManifest struct {
	Modules []struct {
		Key string `json:"Key"`
		Dir string `json:"Dir"`
	} `json:"Modules"`
}

// localModuleDirs returns, sorted and without duplicates, the absolute directory of every module
// the manifest under dataDir records outside the modules directory, root module excluded. A missing
// manifest yields no directories.
func localModuleDirs(fsys vfs.FS, rootDir, dataDir string) ([]string, error) {
	modulesDir := filepath.Join(dataDir, "modules")

	manifestPath := filepath.Join(modulesDir, modulesManifestFile)
	if !vfs.Exists(fsys, manifestPath) {
		return nil, nil
	}

	content, err := vfs.ReadFile(fsys, manifestPath)
	if err != nil {
		return nil, err
	}

	var manifest modulesManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("parsing modules manifest %s: %w", manifestPath, err)
	}

	dirs := make([]string, 0, len(manifest.Modules))

	for _, module := range manifest.Modules {
		if module.Key == "" {
			continue
		}

		dir := module.Dir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(rootDir, dir)
		}

		if isWithin(modulesDir, dir) {
			continue
		}

		dirs = append(dirs, dir)
	}

	slices.Sort(dirs)

	return slices.Compact(dirs), nil
}

// isWithin reports whether path is dir or lies under it.
func isWithin(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}

	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
