// Package rcfile discovers and parses the Terragrunt rc file, a small JSON or YAML file
// that supplies environment variables and CLI flag defaults before Terragrunt parses its
// command line.
//
// The file lets a repository commit the defaults its engineers would otherwise have to
// export by hand, and it is read early enough that variables such as TF_CLI_CONFIG_FILE
// are in the environment before the provider cache server starts.
//
// Note that this package is gated behind the `terragruntrc` experiment, documented in
// /docs/src/data/experiments/terragruntrc.mdx and published at
// /reference/experiments/active#terragruntrc. The file format itself is documented in
// /docs/src/content/docs/04-reference/06-terragruntrc.mdx.
package rcfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// BaseName is the name of the rc file, without the format extension.
	BaseName = ".terragruntrc"

	// ConfigDirName is the directory at the repository root that is searched after the
	// repository root itself.
	ConfigDirName = ".config"

	// AppDirName is the Terragrunt directory inside the user configuration directory.
	AppDirName = "terragrunt"

	// jsonExt is the extension that selects the JSON decoder. Everything else is YAML.
	jsonExt = ".json"

	// gitDirName marks the root of a git repository and bounds the upward search.
	gitDirName = ".git"

	// commandPathSep separates command names in the `name` of a `commands` entry, so that
	// a nested command can be addressed as it is typed: "hcl fmt".
	commandPathSep = " "
)

// FileNames returns the rc file names in the order they are tried within a single
// directory. JSON is tried first, so a directory that happens to hold more than one format
// has a defined outcome.
func FileNames() []string {
	return []string{BaseName + jsonExt, BaseName + ".yaml", BaseName + ".yml"}
}

// Flag is a flag default declared by an rc file.
type Flag struct {
	// Default is the value the flag takes when it is given neither on the command line nor
	// through an environment variable.
	Default any `json:"default" yaml:"default"`

	// Name is the flag name, without leading dashes, as it is typed on the command line.
	Name string `json:"name" yaml:"name"`

	// values holds Default rendered the way the value would be typed on the command line.
	// It is filled in when the file is parsed, so that lookups cannot fail.
	values []string
}

// Command groups the flag defaults that apply to a single command.
type Command struct {
	// Name is the command name, either its own name ("fmt") or its full path from the root
	// command ("hcl fmt").
	Name string `json:"name" yaml:"name"`

	// Flags are the defaults that apply only while this command runs.
	Flags []Flag `json:"flags" yaml:"flags"`
}

// Config is the parsed contents of an rc file.
type Config struct {
	// Env holds environment variables to export before Terragrunt reads its environment.
	Env map[string]string `json:"env" yaml:"env"`

	// Path is the file the configuration was read from.
	Path string `json:"-" yaml:"-"`

	// Flags are the defaults that apply to every command.
	Flags []Flag `json:"flags" yaml:"flags"`

	// Commands are the defaults that apply to a single command.
	Commands []Command `json:"commands" yaml:"commands"`
}

// Find searches for an rc file starting at startDir and returns the path of the first one
// found, or an empty path when there is none, which is the normal case.
//
// Finding a file and reading it are separate steps, so that a caller can name the file it
// found without paying to parse it, and so that a malformed file only fails the runs that
// actually asked for it.
func Find(startDir string) (string, error) {
	dirs, err := SearchDirs(startDir)
	if err != nil {
		return "", err
	}

	fileNames := FileNames()

	for _, dir := range dirs {
		path, err := findInDir(dir, fileNames)
		if err != nil {
			return "", err
		}

		if path != "" {
			return path, nil
		}
	}

	return "", nil
}

// SearchDirs returns the directories that are searched for an rc file, in priority order:
//
//  1. startDir and each parent directory up to and including the git repository root
//  2. the .config directory at the git repository root
//  3. the Terragrunt directory inside the user configuration directory
//  4. the user home directory
//
// When startDir is not inside a git repository, only startDir itself is searched in step 1
// and step 2 is skipped, so that an unrelated ancestor directory cannot configure a run.
func SearchDirs(startDir string) ([]string, error) {
	absDir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", startDir, err)
	}

	dirs, repoRoot := repoDirs(absDir)

	if repoRoot != "" {
		dirs = append(dirs, filepath.Join(repoRoot, ConfigDirName))
	}

	// The user configuration and home directories are only unavailable when the
	// environment has no home directory at all. That is not worth failing a run over, so
	// those locations are skipped instead.
	if userConfigDir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(userConfigDir, AppDirName))
	}

	if homeDir, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, homeDir)
	}

	return dedupe(dirs), nil
}

// dedupe drops repeated directories while keeping the first occurrence of each, so that a
// location which is also the repository root or the home directory is searched once.
func dedupe(dirs []string) []string {
	var (
		seen   = make(map[string]struct{}, len(dirs))
		unique = make([]string, 0, len(dirs))
	)

	for _, dir := range dirs {
		if _, ok := seen[dir]; ok {
			continue
		}

		seen[dir] = struct{}{}
		unique = append(unique, dir)
	}

	return unique
}

// Load reads and parses the rc file at path.
func Load(path string) (*Config, error) {
	// The path is resolved here rather than assumed to be absolute, because values in the
	// `env` section are resolved against the directory holding the file.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", path, err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", absPath, err)
	}

	cfg := &Config{Path: absPath}

	if err := unmarshal(absPath, content, cfg); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// EnvVars returns the environment variables declared by the rc file, with their values
// expanded and their relative paths resolved. See [envValue] for what that means.
func (cfg *Config) EnvVars() map[string]string {
	if cfg == nil || len(cfg.Env) == 0 {
		return nil
	}

	baseDir := filepath.Dir(cfg.Path)
	envVars := make(map[string]string, len(cfg.Env))

	for name, val := range cfg.Env {
		envVars[name] = envValue(baseDir, val)
	}

	return envVars
}

// FlagValues returns the values the rc file declares for a flag, rendered the way they
// would be typed on the command line. A flag that accepts multiple values, such as a slice
// or a map flag, yields one string per declared element.
//
// cmdPath is the chain of commands from the root command down to the command being
// parsed, each entry holding the names that command answers to. It is empty for the global
// flags of the application itself. flagNames are all the names the flag answers to.
//
// The second return value reports whether the rc file declares anything for the flag.
func (cfg *Config) FlagValues(cmdPath [][]string, flagNames []string) ([]string, bool) {
	if cfg == nil {
		return nil, false
	}

	flags := cfg.Flags

	if len(cmdPath) > 0 {
		cmd := cfg.command(cmdPath)
		if cmd == nil {
			return nil, false
		}

		flags = cmd.Flags
	}

	for _, flag := range flags {
		if slices.Contains(flagNames, flag.Name) {
			return flag.values, true
		}
	}

	return nil, false
}

// command returns the entry that addresses the command at the end of cmdPath.
func (cfg *Config) command(cmdPath [][]string) *Command {
	for i, cmd := range cfg.Commands {
		if matchesCommandPath(cmd.Name, cmdPath) {
			return &cfg.Commands[i]
		}
	}

	return nil
}

// matchesCommandPath reports whether a declared command name addresses the command at the
// end of cmdPath. A name is either the command's own name ("format") or its full path as
// it is typed ("hcl format"), and every command also answers to its aliases ("hcl fmt").
func matchesCommandPath(name string, cmdPath [][]string) bool {
	parts := strings.Split(name, commandPathSep)

	if len(parts) == 1 {
		return slices.Contains(cmdPath[len(cmdPath)-1], name)
	}

	if len(parts) != len(cmdPath) {
		return false
	}

	for i, part := range parts {
		if !slices.Contains(cmdPath[i], part) {
			return false
		}
	}

	return true
}

// validate rejects incomplete declarations and renders every declared default, so that a
// mistake in a committed rc file is reported when the file is read rather than ignored.
func (cfg *Config) validate() error {
	if err := validateFlags(cfg.Path, cfg.Flags); err != nil {
		return err
	}

	for i, cmd := range cfg.Commands {
		if cmd.Name == "" {
			return fmt.Errorf("%s: %w", cfg.Path, ErrMissingCommandName)
		}

		if err := validateFlags(cfg.Path, cfg.Commands[i].Flags); err != nil {
			return fmt.Errorf("command %q: %w", cmd.Name, err)
		}
	}

	return nil
}

// validateFlags checks each declaration in flags and fills in its rendered values.
func validateFlags(path string, flags []Flag) error {
	for i, flag := range flags {
		if flag.Name == "" {
			return fmt.Errorf("%s: %w", path, ErrMissingFlagName)
		}

		values, err := flagValues(flag.Default)
		if err != nil {
			return fmt.Errorf("%s: flag %q: %w", path, flag.Name, err)
		}

		flags[i].values = values
	}

	return nil
}

// unmarshal decodes content into cfg with the decoder that matches the file extension.
// Both decoders reject unknown fields, so that a typo in a committed rc file is reported
// instead of silently doing nothing.
func unmarshal(path string, content []byte, cfg *Config) error {
	if len(bytes.TrimSpace(content)) == 0 {
		return nil
	}

	if strings.EqualFold(filepath.Ext(path), jsonExt) {
		dec := json.NewDecoder(bytes.NewReader(content))
		dec.DisallowUnknownFields()

		if err := dec.Decode(cfg); err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}

		return nil
	}

	dec := yaml.NewDecoder(bytes.NewReader(content))
	dec.KnownFields(true)

	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}

	return nil
}

// findInDir returns the path of the first of fileNames present in dir, or an empty string
// when the directory holds none of them.
func findInDir(dir string, fileNames []string) (string, error) {
	for _, name := range fileNames {
		path := filepath.Join(dir, name)

		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}

			return "", fmt.Errorf("failed to check %s: %w", path, err)
		}

		if !info.IsDir() {
			return path, nil
		}
	}

	return "", nil
}

// repoDirs returns dir and its parents up to the git repository root, along with the root
// itself. When no repository root is found, only dir is returned and the root is empty.
func repoDirs(dir string) ([]string, string) {
	var (
		dirs    []string
		current = dir
	)

	for {
		dirs = append(dirs, current)

		if isRepoRoot(current) {
			return dirs, current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return []string{dir}, ""
		}

		current = parent
	}
}

// isRepoRoot reports whether dir holds a .git entry. Worktrees and submodules have a .git
// file rather than a directory, so both are accepted.
func isRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, gitDirName))

	return err == nil
}
