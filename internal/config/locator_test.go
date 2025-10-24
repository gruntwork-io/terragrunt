package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestFindGitRepoRoot tests the git repository root discovery functionality.
func TestFindGitRepoRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T) string // Returns the starting directory
		cleanup   func(t *testing.T, dir string)
		wantFound bool
		wantErr   bool
	}{
		{
			name: "regular git repository with .git directory",
			setup: func(t *testing.T) string {
				t.Helper()
				// Create temp directory structure
				baseDir := t.TempDir()
				gitDir := filepath.Join(baseDir, ".git")
				if err := os.Mkdir(gitDir, 0755); err != nil {
					t.Fatalf("failed to create .git directory: %v", err)
				}
				// Create a subdirectory to search from
				subDir := filepath.Join(baseDir, "sub", "nested")
				if err := os.MkdirAll(subDir, 0755); err != nil {
					t.Fatalf("failed to create subdirectory: %v", err)
				}
				return subDir
			},
			wantFound: true,
			wantErr:   false,
		},
		{
			name: "git worktree with .git file",
			setup: func(t *testing.T) string {
				t.Helper()
				baseDir := t.TempDir()
				gitFile := filepath.Join(baseDir, ".git")
				// Create .git as a file (simulating worktree)
				if err := os.WriteFile(gitFile, []byte("gitdir: /some/path/.git/worktrees/example"), 0644); err != nil {
					t.Fatalf("failed to create .git file: %v", err)
				}
				return baseDir
			},
			wantFound: true,
			wantErr:   false,
		},
		{
			name: "not in a git repository",
			setup: func(t *testing.T) string {
				t.Helper()
				// Just a temp directory with no .git
				return t.TempDir()
			},
			wantFound: false,
			wantErr:   false,
		},
		{
			name: "nested directory in git repo",
			setup: func(t *testing.T) string {
				t.Helper()
				baseDir := t.TempDir()
				gitDir := filepath.Join(baseDir, ".git")
				if err := os.Mkdir(gitDir, 0755); err != nil {
					t.Fatalf("failed to create .git directory: %v", err)
				}
				// Create deeply nested directory
				deepDir := filepath.Join(baseDir, "a", "b", "c", "d", "e")
				if err := os.MkdirAll(deepDir, 0755); err != nil {
					t.Fatalf("failed to create deep directory: %v", err)
				}
				return deepDir
			},
			wantFound: true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			startDir := tt.setup(t)
			if tt.cleanup != nil {
				defer tt.cleanup(t, startDir)
			}

			got, err := findGitRepoRoot(startDir)

			if (err != nil) != tt.wantErr {
				t.Errorf("findGitRepoRoot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantFound && got == "" {
				t.Errorf("findGitRepoRoot() expected to find repo root, got empty string")
			}

			if !tt.wantFound && got != "" {
				t.Errorf("findGitRepoRoot() expected empty string, got %s", got)
			}

			// If we found a repo root, verify it contains .git
			if got != "" {
				gitPath := filepath.Join(got, ".git")
				if _, err := os.Stat(gitPath); err != nil {
					t.Errorf("findGitRepoRoot() returned %s but .git does not exist there", got)
				}
			}
		})
	}
}

// TestFindConfigFile tests the config file discovery functionality.
func TestFindConfigFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(t *testing.T) (cwd string, cleanup func())
		wantFound   bool
		wantErr     bool
		expectInCwd bool // Expect config in CWD
	}{
		{
			name: "config in current working directory",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				tmpDir := t.TempDir()
				configPath := filepath.Join(tmpDir, ConfigFileName)
				if err := os.WriteFile(configPath, []byte(`{"test": true}`), 0644); err != nil {
					t.Fatalf("failed to create config file: %v", err)
				}
				oldCwd, _ := os.Getwd()
				if err := os.Chdir(tmpDir); err != nil {
					t.Fatalf("failed to change directory: %v", err)
				}
				return tmpDir, func() {
					os.Chdir(oldCwd)
				}
			},
			wantFound:   true,
			expectInCwd: true,
			wantErr:     false,
		},
		{
			name: "config in git repo root",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				baseDir := t.TempDir()
				// Create .git directory
				gitDir := filepath.Join(baseDir, ".git")
				if err := os.Mkdir(gitDir, 0755); err != nil {
					t.Fatalf("failed to create .git directory: %v", err)
				}
				// Create config at repo root
				configPath := filepath.Join(baseDir, ConfigFileName)
				if err := os.WriteFile(configPath, []byte(`{"test": true}`), 0644); err != nil {
					t.Fatalf("failed to create config file: %v", err)
				}
				// Create subdirectory to work from
				subDir := filepath.Join(baseDir, "subdir")
				if err := os.Mkdir(subDir, 0755); err != nil {
					t.Fatalf("failed to create subdirectory: %v", err)
				}
				oldCwd, _ := os.Getwd()
				if err := os.Chdir(subDir); err != nil {
					t.Fatalf("failed to change directory: %v", err)
				}
				return subDir, func() {
					os.Chdir(oldCwd)
				}
			},
			wantFound:   true,
			expectInCwd: false,
			wantErr:     false,
		},
		{
			name: "config in .config at repo root",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				baseDir := t.TempDir()
				// Create .git directory
				gitDir := filepath.Join(baseDir, ".git")
				if err := os.Mkdir(gitDir, 0755); err != nil {
					t.Fatalf("failed to create .git directory: %v", err)
				}
				// Create .config directory and config file
				configDir := filepath.Join(baseDir, ".config")
				if err := os.Mkdir(configDir, 0755); err != nil {
					t.Fatalf("failed to create .config directory: %v", err)
				}
				configPath := filepath.Join(configDir, ConfigFileName)
				if err := os.WriteFile(configPath, []byte(`{"test": true}`), 0644); err != nil {
					t.Fatalf("failed to create config file: %v", err)
				}
				// Create subdirectory to work from
				subDir := filepath.Join(baseDir, "subdir")
				if err := os.Mkdir(subDir, 0755); err != nil {
					t.Fatalf("failed to create subdirectory: %v", err)
				}
				oldCwd, _ := os.Getwd()
				if err := os.Chdir(subDir); err != nil {
					t.Fatalf("failed to change directory: %v", err)
				}
				return subDir, func() {
					os.Chdir(oldCwd)
				}
			},
			wantFound:   true,
			expectInCwd: false,
			wantErr:     false,
		},
		{
			name: "no config file found",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				tmpDir := t.TempDir()
				oldCwd, _ := os.Getwd()
				if err := os.Chdir(tmpDir); err != nil {
					t.Fatalf("failed to change directory: %v", err)
				}
				return tmpDir, func() {
					os.Chdir(oldCwd)
				}
			},
			wantFound: false,
			wantErr:   false,
		},
		{
			name: "precedence: CWD over repo root",
			setup: func(t *testing.T) (string, func()) {
				t.Helper()
				baseDir := t.TempDir()
				// Create .git directory
				gitDir := filepath.Join(baseDir, ".git")
				if err := os.Mkdir(gitDir, 0755); err != nil {
					t.Fatalf("failed to create .git directory: %v", err)
				}
				// Create config at repo root
				repoConfig := filepath.Join(baseDir, ConfigFileName)
				if err := os.WriteFile(repoConfig, []byte(`{"location": "repo_root"}`), 0644); err != nil {
					t.Fatalf("failed to create repo config: %v", err)
				}
				// Create subdirectory with its own config
				subDir := filepath.Join(baseDir, "subdir")
				if err := os.Mkdir(subDir, 0755); err != nil {
					t.Fatalf("failed to create subdirectory: %v", err)
				}
				cwdConfig := filepath.Join(subDir, ConfigFileName)
				if err := os.WriteFile(cwdConfig, []byte(`{"location": "cwd"}`), 0644); err != nil {
					t.Fatalf("failed to create cwd config: %v", err)
				}
				oldCwd, _ := os.Getwd()
				if err := os.Chdir(subDir); err != nil {
					t.Fatalf("failed to change directory: %v", err)
				}
				return subDir, func() {
					os.Chdir(oldCwd)
				}
			},
			wantFound:   true,
			expectInCwd: true, // Should find CWD config, not repo root
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: Cannot use t.Parallel() here because we're changing the working directory

			cwd, cleanup := tt.setup(t)
			defer cleanup()

			ctx := context.Background()
			got, err := FindConfigFile(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("FindConfigFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantFound && got == "" {
				t.Errorf("FindConfigFile() expected to find config, got empty string")
			}

			if !tt.wantFound && got != "" {
				t.Errorf("FindConfigFile() expected empty string, got %s", got)
			}

			if tt.expectInCwd && got != "" {
				expectedPath := filepath.Join(cwd, ConfigFileName)
				// Resolve both paths to handle symlinks (e.g., macOS /var -> /private/var)
				gotAbs, err1 := filepath.EvalSymlinks(got)
				expectedAbs, err2 := filepath.EvalSymlinks(expectedPath)
				if err1 == nil && err2 == nil {
					if gotAbs != expectedAbs {
						t.Errorf("FindConfigFile() expected config in CWD %s, got %s", expectedAbs, gotAbs)
					}
				} else if got != expectedPath {
					t.Errorf("FindConfigFile() expected config in CWD %s, got %s", expectedPath, got)
				}
			}

			// If we found a config, verify it exists
			if got != "" {
				if _, err := os.Stat(got); err != nil {
					t.Errorf("FindConfigFile() returned %s but file does not exist", got)
				}
			}
		})
	}
}

// TestFindConfigFileContextCaching tests that the config file path is cached in context.
func TestFindConfigFileContextCaching(t *testing.T) {
	t.Helper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ConfigFileName)
	if err := os.WriteFile(configPath, []byte(`{"test": true}`), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	defer os.Chdir(oldCwd)

	ctx := context.Background()

	// First call - should search and find
	first, err := FindConfigFile(ctx)
	if err != nil {
		t.Fatalf("FindConfigFile() first call error = %v", err)
	}
	if first == "" {
		t.Fatal("FindConfigFile() first call returned empty string")
	}

	// Store the result in context
	ctx = context.WithValue(ctx, configPathKey, first)

	// Second call - should use cached value
	// Delete the file to prove it's using cache, not searching
	if err := os.Remove(configPath); err != nil {
		t.Fatalf("failed to remove config file: %v", err)
	}

	second, err := FindConfigFile(ctx)
	if err != nil {
		t.Fatalf("FindConfigFile() second call error = %v", err)
	}

	if second != first {
		t.Errorf("FindConfigFile() cache mismatch: first=%s, second=%s", first, second)
	}
}

// TestFindConfigFileContextCancellation tests that context cancellation is respected.
func TestFindConfigFileContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := FindConfigFile(ctx)
	if err == nil {
		t.Error("FindConfigFile() expected error for cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Errorf("FindConfigFile() expected context.Canceled, got %v", err)
	}
}

// BenchmarkFindConfigFile benchmarks the config file discovery performance.
func BenchmarkFindConfigFile(b *testing.B) {
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, ConfigFileName)
	if err := os.WriteFile(configPath, []byte(`{"test": true}`), 0644); err != nil {
		b.Fatalf("failed to create config file: %v", err)
	}

	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		b.Fatalf("failed to change directory: %v", err)
	}
	defer os.Chdir(oldCwd)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := FindConfigFile(ctx)
		if err != nil {
			b.Fatalf("FindConfigFile() error = %v", err)
		}
	}
}

// BenchmarkFindConfigFileCached benchmarks cached config file access.
func BenchmarkFindConfigFileCached(b *testing.B) {
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, ConfigFileName)
	if err := os.WriteFile(configPath, []byte(`{"test": true}`), 0644); err != nil {
		b.Fatalf("failed to create config file: %v", err)
	}

	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		b.Fatalf("failed to change directory: %v", err)
	}
	defer os.Chdir(oldCwd)

	// Pre-populate cache
	ctx := context.WithValue(context.Background(), configPathKey, configPath)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := FindConfigFile(ctx)
		if err != nil {
			b.Fatalf("FindConfigFile() error = %v", err)
		}
	}
}

// BenchmarkFindGitRepoRoot benchmarks git repository root discovery.
func BenchmarkFindGitRepoRoot(b *testing.B) {
	baseDir := b.TempDir()
	gitDir := filepath.Join(baseDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		b.Fatalf("failed to create .git directory: %v", err)
	}

	// Create nested directory
	deepDir := filepath.Join(baseDir, "a", "b", "c")
	if err := os.MkdirAll(deepDir, 0755); err != nil {
		b.Fatalf("failed to create deep directory: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := findGitRepoRoot(deepDir)
		if err != nil {
			b.Fatalf("findGitRepoRoot() error = %v", err)
		}
	}
}
