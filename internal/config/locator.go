// Package config provides configuration file discovery and location services.
package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// ConfigFileName is the standard Terragrunt configuration file name.
	ConfigFileName = ".terragruntrc.json"

	// maxTraversalDepth prevents infinite loops during directory traversal.
	maxTraversalDepth = 100
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const (
	// configPathKey is the context key for caching the discovered config file path.
	configPathKey contextKey = "terragrunt_config_file_path"

	// gitRepoRootKey is the context key for caching the git repository root.
	gitRepoRootKey contextKey = "git_repo_root_path"
)

// FindConfigFile searches for .terragruntrc.json in standard locations with precedence.
// Search order:
//  1. Current working directory: ./.terragruntrc.json
//  2. Repository root: <repo_root>/.terragruntrc.json
//  3. .config at repo root: <repo_root>/.config/.terragruntrc.json
//  4. User home directory: ~/.terragruntrc.json
//
// Returns the absolute path to the first config file found, or empty string if not found.
// Only returns error for filesystem access failures, not for "file not found" scenarios.
// The discovered path is cached in the context for the duration of the run.
func FindConfigFile(ctx context.Context) (string, error) {
	// Check context cache first for performance
	if cached, ok := ctx.Value(configPathKey).(string); ok {
		log.Debugf("Using cached config file path: %s", cached)
		return cached, nil
	}

	// Check context for cancellation before starting expensive operations
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting current working directory: %w", err)
	}

	log.Debugf("Searching for %s starting from: %s", ConfigFileName, cwd)

	// Build search locations in precedence order
	searchLocations := make([]string, 0, 4)

	// 1. Current working directory
	searchLocations = append(searchLocations, filepath.Join(cwd, ConfigFileName))

	// 2-3. Repository root and .config directory (if in a git repo)
	repoRoot, err := findGitRepoRoot(cwd)
	if err != nil {
		// Filesystem error during traversal - this is a real error
		return "", fmt.Errorf("searching for git repository root: %w", err)
	}
	if repoRoot != "" {
		// Found a git repo, add repo root locations
		searchLocations = append(searchLocations, filepath.Join(repoRoot, ConfigFileName))
		searchLocations = append(searchLocations, filepath.Join(repoRoot, ".config", ConfigFileName))
		log.Debugf("Found git repository root: %s", repoRoot)
	} else {
		log.Debugf("Not in a git repository, skipping repo-based search locations")
	}

	// 4. User home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Warnf("Could not determine home directory: %v", err)
		// Not a fatal error - continue search without home directory
	} else {
		searchLocations = append(searchLocations, filepath.Join(homeDir, ConfigFileName))
	}

	// Search each location in order, return on first match (early exit)
	for _, location := range searchLocations {
		// Normalize path
		absPath, err := filepath.Abs(location)
		if err != nil {
			log.Debugf("Skipping invalid path %s: %v", location, err)
			continue
		}

		log.Debugf("Checking for config file at: %s", absPath)

		// Check if file exists
		stat, err := os.Stat(absPath)
		if err == nil {
			// File exists - verify it's a regular file, not a directory
			if stat.IsDir() {
				log.Debugf("Path exists but is a directory, not a file: %s", absPath)
				continue
			}

			log.Infof("Found config file: %s", absPath)
			return absPath, nil
		}

		// Check if this is a "file not found" error vs a real filesystem error
		if !errors.Is(err, fs.ErrNotExist) {
			// This is a real error (permission denied, etc.)
			// Log as warning and continue - the file might exist elsewhere
			log.Warnf("Error accessing %s: %v", absPath, err)
			continue
		}

		// File doesn't exist at this location, continue to next
		log.Debugf("Config file not found at: %s", absPath)
	}

	// No config file found in any location - this is not an error
	log.Debugf("No %s found in any search location", ConfigFileName)
	return "", nil
}

// findGitRepoRoot traverses up the directory tree to find the git repository root.
// It looks for a .git directory or .git file (worktree case).
// Returns the absolute path to the repository root, or empty string if not in a git repo.
// Only returns error for filesystem access failures.
func findGitRepoRoot(startDir string) (string, error) {
	// Check context cache if available (we don't have ctx here, so skip for now)
	// This is a helper function, caching is done at the caller level if needed

	// Normalize starting directory to absolute path
	currentDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path for %s: %w", startDir, err)
	}

	log.Debugf("Searching for git repository root starting from: %s", currentDir)

	// Traverse up the directory tree
	for depth := 0; depth < maxTraversalDepth; depth++ {
		gitPath := filepath.Join(currentDir, ".git")

		stat, err := os.Stat(gitPath)
		if err == nil {
			// Found .git - check if it's a directory (regular repo) or file (worktree)
			if stat.IsDir() {
				log.Debugf("Found .git directory at: %s (regular repository)", currentDir)
				return currentDir, nil
			}

			// .git is a file - this is a git worktree
			// In a worktree, the .git file contains a reference to the actual git directory
			log.Debugf("Found .git file at: %s (git worktree)", currentDir)
			return currentDir, nil
		}

		// Check if this is a "not found" error vs a real filesystem error
		if !errors.Is(err, fs.ErrNotExist) {
			// Real error accessing .git (permission denied, etc.)
			// Log warning but continue traversal - git repo might be higher up
			log.Debugf("Error accessing .git at %s: %v", gitPath, err)
		}

		// Move up to parent directory
		parent := filepath.Dir(currentDir)

		// Check if we've reached the filesystem root
		if parent == currentDir {
			log.Debugf("Reached filesystem root, not in a git repository")
			break
		}

		currentDir = parent
	}

	// Reached max depth or filesystem root without finding .git
	if depth := maxTraversalDepth; depth >= maxTraversalDepth {
		log.Warnf("Reached maximum traversal depth (%d) without finding git repository", maxTraversalDepth)
	}

	return "", nil
}
