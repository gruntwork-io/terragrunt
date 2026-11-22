package tf

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

const (
	// InitFingerprintFile is the file in the data directory that records the [InitFingerprint]
	// taken right after the last successful init.
	InitFingerprintFile = "terragrunt-init-fingerprint"

	modulesManifestFile = "modules.json"
	envNameTFVarPrefix  = "TF_VAR_"
)

// InitFingerprint identifies what init reads and what it leaves behind for a root module: the
// configuration files of the root module and of every local module recorded in the modules
// manifest, the lock file, whether the plugins, providers, and modules directories exist, and,
// when the configuration needs them during init, the variable values init sees.
//
// Modules that init downloaded into the modules directory are left out, as nothing but init changes
// them. A recorded local module whose directory is gone hashes differently from one that is present.
type InitFingerprint struct {
	// Files hashes the bytes of every configuration file.
	Files string

	// Config hashes only the parts of the configuration files that init reads: the names of the
	// files, terraform and module blocks, the labels and meta-arguments of provider blocks, and the
	// labels and provider meta-argument of resource, data, and ephemeral blocks. Locals, variables,
	// outputs, and the remaining arguments of those blocks are left out. Any other block counts
	// whole.
	//
	// The bytes of every file count instead when the configuration has static references: one of
	// those parts refers to a variable or a local, which OpenTofu evaluates during init, or a file
	// is not parsed and might. JSON files, files larger than 1 MiB, and files that fail to parse
	// are not parsed.
	Config string

	// Inputs hashes the [InitInputs] when the configuration has static references, and is empty
	// otherwise.
	Inputs string
}

// InitInputs are what init reads variable values from besides the configuration files.
type InitInputs struct {
	// Env is the environment init runs with. Of it, the TF_VAR_ variables, TF_CLI_ARGS, and
	// TF_CLI_ARGS_init count.
	Env map[string]string

	// Args are the arguments init runs with. The content of the file each -var-file names counts
	// too, read relative to the root module.
	Args []string

	// Generated holds the variable values that Terragrunt passes in var files it writes itself.
	Generated []byte

	// GeneratedVarFiles names the var files in the root module that Terragrunt writes itself.
	// They are left out of the auto-loaded var files that count, as Generated stands in for them.
	GeneratedVarFiles []string
}

// ComputeInitFingerprint returns the [InitFingerprint] of the root module at rootDir with the data
// directory dataDir. It calls inputs only when the configuration has static references.
func ComputeInitFingerprint(
	fsys vfs.FS,
	rootDir, dataDir string,
	inputs func() (InitInputs, error),
) (InitFingerprint, error) {
	fp := newFingerprinter(fsys, rootDir, hashFilesAndConfig)
	if err := fp.hashModule(dataDir); err != nil {
		return InitFingerprint{}, err
	}

	fingerprint := InitFingerprint{Files: fp.filesSum(), Config: fp.configSum()}
	if !fp.staticRefs {
		return fingerprint, nil
	}

	inputsSum, err := hashInitInputs(fsys, rootDir, inputs)
	if err != nil {
		return InitFingerprint{}, err
	}

	fingerprint.Inputs = inputsSum

	return fingerprint, nil
}

// InitFingerprintCurrent reports whether the root module at rootDir with the data directory dataDir
// still has the fingerprint recorded. It hashes the bytes of the configuration files first, and
// parses them only when those differ from recorded. It calls inputs only when the configuration
// has static references.
func InitFingerprintCurrent(
	fsys vfs.FS,
	rootDir, dataDir string,
	recorded InitFingerprint,
	inputs func() (InitInputs, error),
) (bool, error) {
	files := newFingerprinter(fsys, rootDir, hashFiles)
	if err := files.hashModule(dataDir); err != nil {
		return false, err
	}

	staticRefs := recorded.Inputs != ""

	if files.filesSum() != recorded.Files {
		config := newFingerprinter(fsys, rootDir, hashFilesAndConfig)
		if err := config.hashModule(dataDir); err != nil {
			return false, err
		}

		if config.configSum() != recorded.Config || config.staticRefs != staticRefs {
			return false, nil
		}
	}

	if !staticRefs {
		return true, nil
	}

	inputsSum, err := hashInitInputs(fsys, rootDir, inputs)
	if err != nil {
		return false, err
	}

	return inputsSum == recorded.Inputs, nil
}

// WriteInitFingerprint records fingerprint in [InitFingerprintFile] under dataDir, creating the
// directory when init left none behind.
func WriteInitFingerprint(fsys vfs.FS, dataDir string, fingerprint InitFingerprint) error {
	const dirPerms = 0o755

	if err := fsys.MkdirAll(dataDir, dirPerms); err != nil {
		return err
	}

	const filePerms = 0o644

	lines := []string{fingerprint.Files, fingerprint.Config, fingerprint.Inputs}
	content := strings.Join(lines, "\n") + "\n"

	return vfs.WriteFile(fsys, filepath.Join(dataDir, InitFingerprintFile), []byte(content), filePerms)
}

// ReadInitFingerprint returns the fingerprint that [WriteInitFingerprint] recorded under dataDir.
// Content in any other format yields a fingerprint that no root module has.
//
// Returns an error matching [os.ErrNotExist] when no fingerprint is recorded.
func ReadInitFingerprint(fsys vfs.FS, dataDir string) (InitFingerprint, error) {
	content, err := vfs.ReadFile(fsys, filepath.Join(dataDir, InitFingerprintFile))
	if err != nil {
		return InitFingerprint{}, err
	}

	const fields = 3

	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	if len(lines) != fields {
		return InitFingerprint{}, nil
	}

	return InitFingerprint{Files: lines[0], Config: lines[1], Inputs: lines[2]}, nil
}

// hashInitInputs returns the hash of the [InitInputs] that inputs returns for the root module at
// rootDir.
func hashInitInputs(
	fsys vfs.FS,
	rootDir string,
	inputs func() (InitInputs, error),
) (string, error) {
	in, err := inputs()
	if err != nil {
		return "", err
	}

	h := sha256.New()
	entry := []byte{}

	hashEntry := func(label, value string) {
		entry = appendEntry(entry[:0], label, value)
		h.Write(entry)
	}

	for _, name := range slices.Sorted(maps.Keys(in.Env)) {
		if isInitEnv(name) {
			hashEntry("env", name)
			hashEntry("value", in.Env[name])
		}
	}

	for _, arg := range in.Args {
		hashEntry("arg", arg)
	}

	for _, path := range varFileArgs(in.Args) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(rootDir, path)
		}

		if err := hashVarFile(fsys, hashEntry, path); err != nil {
			return "", err
		}
	}

	hashEntry("generated", string(in.Generated))

	autoLoaded, err := autoLoadedVarFiles(fsys, rootDir, in.GeneratedVarFiles)
	if err != nil {
		return "", err
	}

	for _, path := range autoLoaded {
		hashEntry("auto", filepath.Base(path))

		if err := hashVarFile(fsys, hashEntry, path); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// isInitEnv reports whether the environment variable name can change what init does: a `TF_VAR_`
// variable sets a value the configuration reads while it loads, and `TF_CLI_ARGS` and
// `TF_CLI_ARGS_init` add arguments to init.
func isInitEnv(name string) bool {
	return strings.HasPrefix(name, envNameTFVarPrefix) ||
		name == EnvNameTFCLIArgs ||
		name == EnvNameTFCLIArgsInit
}

// hashVarFile passes to hashEntry the hash of the var file at path, or a marker when it is missing.
func hashVarFile(fsys vfs.FS, hashEntry func(label, value string), path string) error {
	if !vfs.Exists(fsys, path) {
		hashEntry("var-file", "missing")

		return nil
	}

	h := sha256.New()
	if err := vfs.HashFile(fsys, h, path); err != nil {
		return err
	}

	hashEntry("var-file", hex.EncodeToString(h.Sum(nil)))

	return nil
}

// varFileArgs returns the paths that the -var-file arguments in args name, in either the
// "-var-file=path" or the "-var-file path" form.
func varFileArgs(args []string) []string {
	var paths []string

	for i, arg := range args {
		if path, ok := strings.CutPrefix(arg, "-var-file="); ok {
			paths = append(paths, path)

			continue
		}

		if arg == "-var-file" && i+1 < len(args) {
			paths = append(paths, args[i+1])
		}
	}

	return paths
}

// autoLoadedVarFiles returns, sorted, the var files directly under rootDir that OpenTofu loads
// without being asked, leaving out those named in skip.
func autoLoadedVarFiles(fsys vfs.FS, rootDir string, skip []string) ([]string, error) {
	entries, err := vfs.ReadDir(fsys, rootDir)
	if err != nil {
		return nil, err
	}

	var paths []string

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || slices.Contains(skip, name) {
			continue
		}

		switch {
		case name == "terraform.tfvars", name == "terraform.tfvars.json",
			strings.HasSuffix(name, ".auto.tfvars"), strings.HasSuffix(name, ".auto.tfvars.json"):
			paths = append(paths, filepath.Join(rootDir, name))
		}
	}

	slices.Sort(paths)

	return paths, nil
}

// fingerprintMode selects which hashes a fingerprinter computes.
type fingerprintMode int

const (
	// hashFiles computes [InitFingerprint.Files] only.
	hashFiles fingerprintMode = iota

	// hashFilesAndConfig computes both hashes of an [InitFingerprint].
	hashFilesAndConfig
)

// fingerprinter accumulates one [InitFingerprint].
type fingerprinter struct {
	// filesHash accumulates [InitFingerprint.Files].
	filesHash hash.Hash

	// configHash accumulates [InitFingerprint.Config], and goes unused under [hashFiles].
	configHash hash.Hash

	// fsys is where the root module and its local modules live.
	fsys vfs.FS

	// rootDir is the directory of the root module.
	rootDir string

	// entry is the buffer that every entry is built in before it is hashed.
	entry []byte

	// mode selects the hashes to compute.
	mode fingerprintMode

	// staticRefs records whether a part of a file that configHash covers refers to a variable or a
	// local.
	staticRefs bool
}

// newFingerprinter returns a fingerprinter for the root module at rootDir that computes the hashes
// mode selects.
func newFingerprinter(fsys vfs.FS, rootDir string, mode fingerprintMode) *fingerprinter {
	return &fingerprinter{
		filesHash:  sha256.New(),
		configHash: sha256.New(),
		fsys:       fsys,
		rootDir:    rootDir,
		mode:       mode,
	}
}

// hashModule folds into the hashes everything an [InitFingerprint] covers.
func (fp *fingerprinter) hashModule(dataDir string) error {
	if err := fp.hashConfigDir(fp.rootDir); err != nil {
		return err
	}

	moduleDirs, err := localModuleDirs(fp.fsys, fp.rootDir, dataDir)
	if err != nil {
		return err
	}

	for _, dir := range moduleDirs {
		if !vfs.Exists(fp.fsys, dir) {
			fp.hashEntry("missing", relSlash(fp.rootDir, dir))

			continue
		}

		if err := fp.hashConfigDir(dir); err != nil {
			return err
		}
	}

	if err := fp.hashLockFile(); err != nil {
		return err
	}

	for _, name := range []string{"plugins", "providers", "modules"} {
		fp.hashEntry(name, strconv.FormatBool(vfs.Exists(fp.fsys, filepath.Join(dataDir, name))))
	}

	return nil
}

// filesSum returns [InitFingerprint.Files] once [fingerprinter.hashModule] has run.
func (fp *fingerprinter) filesSum() string {
	return hex.EncodeToString(fp.filesHash.Sum(nil))
}

// configSum returns [InitFingerprint.Config] once [fingerprinter.hashModule] has run under
// [hashFilesAndConfig].
func (fp *fingerprinter) configSum() string {
	if fp.staticRefs {
		return fp.filesSum()
	}

	return hex.EncodeToString(fp.configHash.Sum(nil))
}

// hashConfigDir folds into the hashes the configuration files directly under dir.
func (fp *fingerprinter) hashConfigDir(dir string) error {
	fp.hashEntry("dir", relSlash(fp.rootDir, dir))

	files, err := util.ListTfFiles(fp.fsys, dir)
	if err != nil {
		return err
	}

	for _, path := range files {
		fp.hashEntry("file", relSlash(fp.rootDir, path))

		if err := fp.hashConfigFile(path); err != nil {
			return err
		}
	}

	return nil
}

// hashConfigFile folds the configuration file at path into the hashes.
func (fp *fingerprinter) hashConfigFile(path string) error {
	if fp.mode == hashFiles {
		return fp.hashFileBytes(path)
	}

	const maxParsedFileSize = 1 << 20

	info, err := fp.fsys.Stat(path)
	if err != nil {
		return err
	}

	if strings.HasSuffix(path, ".json") || info.Size() > maxParsedFileSize {
		fp.staticRefs = true

		return fp.hashFileBytes(path)
	}

	src, err := vfs.ReadFile(fp.fsys, path)
	if err != nil {
		return err
	}

	rawSum := sha256.Sum256(src)
	sum := hex.EncodeToString(rawSum[:])

	fp.hashFilesEntry("sha256", sum)

	file, diags := hclsyntax.ParseConfig(src, path, hcl.InitialPos)
	if diags.HasErrors() {
		fp.staticRefs = true
		fp.hashConfigEntry("sha256", sum)

		return nil
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		panic(fmt.Sprintf("hclsyntax.ParseConfig returned a %T body", file.Body))
	}

	fp.hashInitParts(src, body)

	return nil
}

// hashFileBytes folds the bytes of the file at path into both hashes.
func (fp *fingerprinter) hashFileBytes(path string) error {
	sum, err := fp.fileSum(path)
	if err != nil {
		return err
	}

	fp.hashEntry("sha256", sum)

	return nil
}

// hashInitParts folds into configHash the parts of the top-level body of src that init reads.
func (fp *fingerprinter) hashInitParts(src []byte, body *hclsyntax.Body) {
	for _, name := range slices.Sorted(maps.Keys(body.Attributes)) {
		fp.hashConfigEntry("attribute", string(rangeBytes(src, body.Attributes[name].Range())))
	}

	for _, block := range body.Blocks {
		switch block.Type {
		case "locals", "variable", "output":
		case "provider":
			fp.hashProviderBlock(src, block)
		case "resource", "data", "ephemeral":
			fp.hashBlockHeader(block)
			fp.hashArg(src, block, "provider")
		case "terraform":
			fp.staticRefs = fp.staticRefs || refersToStaticInput(block.Body)

			fp.hashConfigEntry("block", string(rangeBytes(src, block.Range())))
		case "module":
			fp.noteStaticRefs(block, "source", "version")

			fp.hashConfigEntry("block", string(rangeBytes(src, block.Range())))
		default:
			fp.hashConfigEntry("block", string(rangeBytes(src, block.Range())))
		}
	}
}

// hashProviderBlock folds into configHash the header of a provider block and the meta-arguments
// and reserved names that OpenTofu reads from it, following providerBlockSchema in OpenTofu's
// internal/configs/provider.go. The provider's own settings, including those in a "_" block, are
// left out.
func (fp *fingerprinter) hashProviderBlock(src []byte, block *hclsyntax.Block) {
	fp.hashBlockHeader(block)

	for _, name := range []string{"alias", "version", "for_each", "count", "depends_on", "source"} {
		fp.hashArg(src, block, name)
	}

	fp.noteStaticRefs(block, "for_each")

	for _, nested := range block.Body.Blocks {
		switch nested.Type {
		case "lifecycle", "locals":
			fp.hashConfigEntry("block", string(rangeBytes(src, nested.Range())))
		}
	}
}

// hashBlockHeader folds the type and labels of block into configHash.
func (fp *fingerprinter) hashBlockHeader(block *hclsyntax.Block) {
	fp.hashConfigEntry("type", block.Type)

	for _, label := range block.Labels {
		fp.hashConfigEntry("label", label)
	}
}

// hashArg folds the expression of the argument name of block, if set, into configHash.
func (fp *fingerprinter) hashArg(src []byte, block *hclsyntax.Block, name string) {
	attr, ok := block.Body.Attributes[name]
	if !ok {
		return
	}

	fp.hashConfigEntry(name, string(rangeBytes(src, attr.Expr.Range())))
}

// noteStaticRefs records in staticRefs whether one of the named arguments of block, which
// OpenTofu evaluates during init, refers to a variable or a local.
func (fp *fingerprinter) noteStaticRefs(block *hclsyntax.Block, names ...string) {
	for _, name := range names {
		attr, ok := block.Body.Attributes[name]
		if ok && refersToStaticInput(attr.Expr) {
			fp.staticRefs = true
		}
	}
}

func (fp *fingerprinter) hashLockFile() error {
	path := filepath.Join(fp.rootDir, TerraformLockFile)
	if !vfs.Exists(fp.fsys, path) {
		fp.hashEntry("lock", "absent")

		return nil
	}

	sum, err := fp.fileSum(path)
	if err != nil {
		return err
	}

	fp.hashEntry("lock", sum)

	return nil
}

// fileSum returns the hex SHA-256 digest of the file at path.
func (fp *fingerprinter) fileSum(path string) (string, error) {
	h := sha256.New()
	if err := vfs.HashFile(fp.fsys, h, path); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashEntry folds one labeled value into both hashes.
func (fp *fingerprinter) hashEntry(label, value string) {
	fp.hashFilesEntry(label, value)

	if fp.mode == hashFilesAndConfig {
		fp.hashConfigEntry(label, value)
	}
}

// hashFilesEntry folds one labeled value into filesHash with a length prefix, so that two
// different sequences of entries never hash alike.
func (fp *fingerprinter) hashFilesEntry(label, value string) {
	fp.entry = appendEntry(fp.entry[:0], label, value)
	fp.filesHash.Write(fp.entry)
}

// hashConfigEntry is [fingerprinter.hashFilesEntry] for configHash.
func (fp *fingerprinter) hashConfigEntry(label, value string) {
	fp.entry = appendEntry(fp.entry[:0], label, value)
	fp.configHash.Write(fp.entry)
}

// appendEntry appends to dst the entry for one labeled value.
func appendEntry(dst []byte, label, value string) []byte {
	const decimal = 10

	dst = append(dst, label...)
	dst = append(dst, ':')
	dst = strconv.AppendInt(dst, int64(len(value)), decimal)
	dst = append(dst, ':')
	dst = append(dst, value...)

	return append(dst, '\n')
}

// rangeBytes returns the bytes of src that r covers.
func rangeBytes(src []byte, r hcl.Range) []byte {
	return src[r.Start.Byte:r.End.Byte]
}

// refersToStaticInput reports whether node refers to a variable or a local, the values OpenTofu
// evaluates while it loads the configuration for init.
func refersToStaticInput(node hclsyntax.Node) bool {
	refs := hclsyntax.VisitAll(node, func(n hclsyntax.Node) hcl.Diagnostics {
		traversal, ok := n.(*hclsyntax.ScopeTraversalExpr)
		if !ok {
			return nil
		}

		switch traversal.Traversal.RootName() {
		case "var", "local":
			return hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "static input",
				Subject:  &traversal.SrcRange,
			}}
		}

		return nil
	})

	return len(refs) > 0
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
