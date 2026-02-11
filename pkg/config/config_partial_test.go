package config_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestPartialParseResolvesLocals(t *testing.T) {
	t.Parallel()

	cfg := `
locals {
  app1 = "../app1"
}

dependencies {
  paths = [local.app1]
}
`

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependenciesBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
	assert.Equal(t, "../app1", terragruntConfig.Dependencies.Paths[0])
	assert.Equal(t, map[string]any{"app1": "../app1"}, terragruntConfig.Locals)

	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
}

func TestPartialParseDoesNotResolveIgnoredBlock(t *testing.T) {
	t.Parallel()

	cfg := `
dependencies {
  # This function call will fail when attempting to decode
  paths = [file("i-am-a-file-that-does-not-exist")]
}

prevent_destroy = false
`

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	_, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)

	pctx = pctx.WithDecodeList(config.DependenciesBlock)
	_, err = config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	assert.Error(t, err)
}

func TestPartialParseMultipleItems(t *testing.T) {
	t.Parallel()

	cfg := `
dependencies {
  paths = ["../app1"]
}

prevent_destroy = true
skip = true
`

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependenciesBlock, config.TerragruntFlags)
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
	assert.Equal(t, "../app1", terragruntConfig.Dependencies.Paths[0])

	assert.True(t, *terragruntConfig.PreventDestroy)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseOmittedItems(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependenciesBlock, config.TerragruntFlags)
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, "", nil)

	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseDoesNotResolveIgnoredBlockEvenInParent(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTestWithConfigPath(t, "../../test/fixtures/partial-parse/ignore-bad-block-in-parent/child/"+config.DefaultTerragruntConfigPath)

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, opts)
	pctx = pctx.WithDecodeList(config.TerragruntFlags)
	_, err := config.PartialParseConfigFile(ctx, pctx, l, opts.TerragruntConfigPath, nil)
	require.NoError(t, err)

	pctx = pctx.WithDecodeList(config.DependenciesBlock)
	_, err = config.PartialParseConfigFile(ctx, pctx, l, opts.TerragruntConfigPath, nil)
	assert.Error(t, err)
}

func TestPartialParseOnlyInheritsSelectedBlocksFlags(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTestWithConfigPath(t, "../../test/fixtures/partial-parse/partial-inheritance/child/"+config.DefaultTerragruntConfigPath)

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, opts)
	pctx = pctx.WithDecodeList(config.TerragruntFlags)
	terragruntConfig, err := config.PartialParseConfigFile(ctx, pctx, l, opts.TerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.True(t, terragruntConfig.IsPartial)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.True(t, *terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseOnlyInheritsSelectedBlocksDependencies(t *testing.T) {
	t.Parallel()

	opts := mockOptionsForTestWithConfigPath(t, "../../test/fixtures/partial-parse/partial-inheritance/child/"+config.DefaultTerragruntConfigPath)

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, opts)
	pctx = pctx.WithDecodeList(config.DependenciesBlock)
	terragruntConfig, err := config.PartialParseConfigFile(ctx, pctx, l, opts.TerragruntConfigPath, nil)
	require.NoError(t, err)

	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
	assert.Equal(t, "../app1", terragruntConfig.Dependencies.Paths[0])

	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseDependencyBlockSetsTerragruntDependencies(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}
`

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependencyBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.TerragruntDependencies)
	assert.Len(t, terragruntConfig.TerragruntDependencies, 1)
	assert.Equal(t, "vpc", terragruntConfig.TerragruntDependencies[0].Name)
	assert.Equal(t, cty.StringVal("../app1"), terragruntConfig.TerragruntDependencies[0].ConfigPath)
}

func TestPartialParseMultipleDependencyBlockSetsTerragruntDependencies(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}

dependency "sql" {
  config_path = "../db1"
}
`

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependencyBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.TerragruntDependencies)
	assert.Len(t, terragruntConfig.TerragruntDependencies, 2)
	assert.Equal(t, "vpc", terragruntConfig.TerragruntDependencies[0].Name)
	assert.Equal(t, cty.StringVal("../app1"), terragruntConfig.TerragruntDependencies[0].ConfigPath)
	assert.Equal(t, "sql", terragruntConfig.TerragruntDependencies[1].Name)
	assert.Equal(t, cty.StringVal("../db1"), terragruntConfig.TerragruntDependencies[1].ConfigPath)
}

func TestPartialParseDependencyBlockSetsDependencies(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}

dependency "sql" {
  config_path = "../db1"
}
`

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependencyBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 2)
	assert.Equal(t, []string{"../app1", "../db1"}, terragruntConfig.Dependencies.Paths)
}

func TestPartialParseDependencyBlockMergesDependencies(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}

dependencies {
  paths = ["../vpc"]
}

dependency "sql" {
  config_path = "../db1"
}
`

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependenciesBlock, config.DependencyBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 3)
	assert.Equal(t, []string{"../vpc", "../app1", "../db1"}, terragruntConfig.Dependencies.Paths)
}

func TestPartialParseDependencyBlockMergesDependenciesOrdering(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}

dependencies {
  paths = ["../vpc"]
}

dependency "sql" {
  config_path = "../db1"
}
`

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependencyBlock, config.DependenciesBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 3)
	assert.Equal(t, []string{"../app1", "../db1", "../vpc"}, terragruntConfig.Dependencies.Paths)
}

func TestPartialParseDependencyBlockMergesDependenciesDedup(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}

dependencies {
  paths = ["../app1"]
}

dependency "sql" {
  config_path = "../db1"
}
`

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependencyBlock, config.DependenciesBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 2)
	assert.Equal(t, []string{"../app1", "../db1"}, terragruntConfig.Dependencies.Paths)
}

func TestPartialParseOnlyParsesTerraformSource(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../vpc"
}

terraform {
  source = "../../modules/app"
  before_hook "before" {
    commands = ["apply"]
	execute  = ["echo", dependency.vpc.outputs.vpc_id]
  }
}
`

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.TerraformSource)
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.True(t, terragruntConfig.IsPartial)

	assert.NotNil(t, terragruntConfig.Terraform)
	assert.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "../../modules/app", *terragruntConfig.Terraform.Source)
}

func TestOptionalDependenciesAreSkipped(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../vpc"
}
dependency "ec2" {
  config_path = "../ec2"
  enabled    = false
}
`

	l := logger.CreateLogger()

	ctx, pctx := config.NewParsingContext(t.Context(), l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependencyBlock)
	terragruntConfig, err := config.PartialParseConfigString(ctx, pctx, l, config.DefaultTerragruntConfigPath, cfg, nil)
	require.NoError(t, err)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
}

func TestPartialParseSavesToHclCache(t *testing.T) {
	t.Parallel()

	// Setup test environment
	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")
	configContent := `dependencies { paths = ["../app1"] }` //nolint:goconst
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

	// Get file metadata for cache key generation
	fileInfo, err := os.Stat(configPath)
	require.NoError(t, err)

	expectedCacheKey := fmt.Sprintf("configPath-%v-modTime-%v", configPath, fileInfo.ModTime().UnixMicro())

	// Setup cache and context
	hclCache := cache.NewCache[*hclparse.File]("test-hcl-cache")
	baseCtx := context.WithValue(t.Context(), config.HclCacheContextKey, hclCache)
	l := logger.CreateLogger()
	ctx, pctx := config.NewParsingContext(baseCtx, l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependenciesBlock)

	// Verify cache is empty initially
	_, found := hclCache.Get(ctx, expectedCacheKey)
	require.False(t, found, "cache should be empty before parsing")

	// Parse config file (should populate cache)
	_, err = config.PartialParseConfigFile(ctx, pctx, l, configPath, nil)
	require.NoError(t, err)

	// Verify file was cached
	cachedFile, found := hclCache.Get(ctx, expectedCacheKey)
	require.True(t, found, "expected file to be in cache after first parse")
	require.NotNil(t, cachedFile, "cached file should not be nil")

	// Verify cached content matches the original
	assert.Equal(t, configPath, cachedFile.ConfigPath)
	assert.Contains(t, cachedFile.Content(), "dependencies")
}

func TestPartialParseCacheHitOnSecondParse(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")
	configContent := `dependencies { paths = ["../app1"] }`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

	fileInfo, err := os.Stat(configPath)
	require.NoError(t, err)

	cacheKey := fmt.Sprintf("configPath-%v-modTime-%v", configPath, fileInfo.ModTime().UnixMicro())

	hclCache := cache.NewCache[*hclparse.File]("test-hcl-cache")
	baseCtx := context.WithValue(t.Context(), config.HclCacheContextKey, hclCache)
	l := logger.CreateLogger()
	ctx, pctx := config.NewParsingContext(baseCtx, l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependenciesBlock)

	// First parse - should be cache miss
	_, err = config.PartialParseConfigFile(ctx, pctx, l, configPath, nil)
	require.NoError(t, err)

	// Verify cache hit on second parse
	_, err = config.PartialParseConfigFile(ctx, pctx, l, configPath, nil)
	require.NoError(t, err)

	// Verify same file object is returned from cache
	cachedFile, found := hclCache.Get(ctx, cacheKey)
	require.True(t, found)
	require.NotNil(t, cachedFile)
}

func TestPartialParseCacheInvalidationOnFileModification(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")
	originalContent := `dependencies { paths = ["../app1"] }`
	modifiedContent := `dependencies { paths = ["../app1", "../app2"] }`

	require.NoError(t, os.WriteFile(configPath, []byte(originalContent), 0644))

	fileInfo, err := os.Stat(configPath)
	require.NoError(t, err)

	originalCacheKey := fmt.Sprintf("configPath-%v-modTime-%v", configPath, fileInfo.ModTime().UnixMicro())

	hclCache := cache.NewCache[*hclparse.File]("test-hcl-cache")
	baseCtx := context.WithValue(t.Context(), config.HclCacheContextKey, hclCache)
	l := logger.CreateLogger()
	ctx, pctx := config.NewParsingContext(baseCtx, l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependenciesBlock)

	// Parse original file
	_, err = config.PartialParseConfigFile(ctx, pctx, l, configPath, nil)
	require.NoError(t, err)

	// Verify original file is cached
	_, found := hclCache.Get(ctx, originalCacheKey)
	require.True(t, found, "original file should be cached")

	// Modify file (this changes mod time)
	require.NoError(t, os.WriteFile(configPath, []byte(modifiedContent), 0644))
	forceModTimeChange(t, configPath, fileInfo.ModTime())

	// Parse modified file - should create new cache entry
	_, err = config.PartialParseConfigFile(ctx, pctx, l, configPath, nil)
	require.NoError(t, err)

	// Verify old cache entry is still there but new one exists
	_, found = hclCache.Get(ctx, originalCacheKey)
	require.True(t, found, "original cache entry should still exist")

	// Get new cache key
	fileInfo, err = os.Stat(configPath)
	require.NoError(t, err)

	newCacheKey := fmt.Sprintf("configPath-%v-modTime-%v", configPath, fileInfo.ModTime().UnixMicro())

	// Verify new file is cached with different content
	newCachedFile, found := hclCache.Get(ctx, newCacheKey)
	require.True(t, found, "modified file should be cached")
	require.NotNil(t, newCachedFile)
	assert.Contains(t, newCachedFile.Content(), "../app2")
}

func TestPartialParseCacheWithInvalidFile(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")
	invalidContent := `invalid hcl syntax {`
	require.NoError(t, os.WriteFile(configPath, []byte(invalidContent), 0644))

	hclCache := cache.NewCache[*hclparse.File]("test-hcl-cache")
	baseCtx := context.WithValue(t.Context(), config.HclCacheContextKey, hclCache)
	l := logger.CreateLogger()
	ctx, pctx := config.NewParsingContext(baseCtx, l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependenciesBlock)

	// Parse should fail and not cache an invalid file
	_, err := config.PartialParseConfigFile(ctx, pctx, l, configPath, nil)
	require.Error(t, err, "parsing invalid HCL should fail")

	// Verify nothing was cached
	fileInfo, err := os.Stat(configPath)
	require.NoError(t, err)

	cacheKey := fmt.Sprintf("configPath-%v-modTime-%v", configPath, fileInfo.ModTime().UnixMicro())

	_, found := hclCache.Get(ctx, cacheKey)
	require.False(t, found, "invalid file should not be cached")
}

func TestPartialParseCacheKeyFormat(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")
	configContent := `dependencies { paths = ["../app1"] }`
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

	fileInfo, err := os.Stat(configPath)
	require.NoError(t, err)

	expectedCacheKey := fmt.Sprintf("configPath-%v-modTime-%v", configPath, fileInfo.ModTime().UnixMicro())

	hclCache := cache.NewCache[*hclparse.File]("test-hcl-cache")
	baseCtx := context.WithValue(t.Context(), config.HclCacheContextKey, hclCache)
	l := logger.CreateLogger()
	ctx, pctx := config.NewParsingContext(baseCtx, l, mockOptionsForTest(t))
	pctx = pctx.WithDecodeList(config.DependenciesBlock)

	_, err = config.PartialParseConfigFile(ctx, pctx, l, configPath, nil)
	require.NoError(t, err)

	// Verify cache key format matches the expected pattern
	assert.Regexp(t, `^configPath-.*-modTime-\d+$`, expectedCacheKey, "cache key should match expected format")
	assert.Contains(t, expectedCacheKey, configPath, "cache key should contain config path")
	assert.Contains(t, expectedCacheKey, strconv.FormatInt(fileInfo.ModTime().UnixMicro(), 10), "cache key should contain mod time")

	// Verify we can retrieve using the expected key
	_, found := hclCache.Get(ctx, expectedCacheKey)
	require.True(t, found, "should be able to retrieve using expected cache key format")
}

// forceModTimeChange ensures the file at path has a modification time strictly after prev.
func forceModTimeChange(t *testing.T, path string, prev time.Time) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err := os.Chtimes(path, time.Now(), time.Now())

		require.NoError(t, err)

		if fileInfo, err := os.Stat(path); err == nil && fileInfo.ModTime().After(prev) {
			return
		}

		time.Sleep(1 * time.Millisecond)
	}

	t.Fatalf("Failed to change modification time of %s within 5 seconds", path)
}

// TestPartialParseConfigCacheDifferentCallers verifies that the partial parse config cache
// creates separate entries for different calling modules parsing the same file.
// This prevents cross-environment dependency bugs where context-sensitive functions
// (e.g. path_relative_to_include) return wrong values from a cached result.
func TestPartialParseConfigCacheDifferentCallers(t *testing.T) {
	t.Parallel()

	// Create a shared config file that both modules will parse.
	tmpDir := helpers.TmpDirWOSymlinks(t)
	sharedConfigPath := filepath.Join(tmpDir, "shared.hcl")
	sharedContent := `dependencies { paths = ["../app1"] }`
	require.NoError(t, os.WriteFile(sharedConfigPath, []byte(sharedContent), 0644))

	// Create two different module directories with distinct config paths.
	moduleADir := filepath.Join(tmpDir, "moduleA")
	moduleBDir := filepath.Join(tmpDir, "moduleB")

	require.NoError(t, os.MkdirAll(moduleADir, 0755))
	require.NoError(t, os.MkdirAll(moduleBDir, 0755))

	moduleAConfigPath := filepath.Join(moduleADir, "terragrunt.hcl")
	moduleBConfigPath := filepath.Join(moduleBDir, "terragrunt.hcl")

	require.NoError(t, os.WriteFile(moduleAConfigPath, []byte(""), 0644))
	require.NoError(t, os.WriteFile(moduleBConfigPath, []byte(""), 0644))

	// Setup shared caches in context so both modules use the same config cache.
	hclCache := cache.NewCache[*hclparse.File]("test-hcl-cache")
	configCache := cache.NewCache[*config.TerragruntConfig]("test-config-cache")
	baseCtx := context.WithValue(t.Context(), config.HclCacheContextKey, hclCache)
	baseCtx = context.WithValue(baseCtx, config.TerragruntConfigCacheContextKey, configCache)
	l := logger.CreateLogger()

	// Parse shared config from module A's context.
	optsA := mockOptionsForTestWithConfigPath(t, moduleAConfigPath)
	optsA.UsePartialParseConfigCache = true
	ctxA, pctxA := config.NewParsingContext(baseCtx, l, optsA)
	pctxA = pctxA.WithDecodeList(config.DependenciesBlock)

	configA, err := config.PartialParseConfigFile(ctxA, pctxA, l, sharedConfigPath, nil)
	require.NoError(t, err)
	require.NotNil(t, configA)

	// Parse shared config from module B's context (different TerragruntConfigPath).
	optsB := mockOptionsForTestWithConfigPath(t, moduleBConfigPath)
	optsB.UsePartialParseConfigCache = true
	ctxB, pctxB := config.NewParsingContext(baseCtx, l, optsB)
	pctxB = pctxB.WithDecodeList(config.DependenciesBlock)

	configB, err := config.PartialParseConfigFile(ctxB, pctxB, l, sharedConfigPath, nil)
	require.NoError(t, err)
	require.NotNil(t, configB)

	// Verify that two separate cache entries were created (one per caller),
	// not a single shared entry. This ensures context-sensitive functions like
	// path_relative_to_include() would evaluate correctly for each caller.
	configCache.Mutex.RLock()
	cacheLen := len(configCache.Cache)
	configCache.Mutex.RUnlock()
	assert.Equal(t, 2, cacheLen, "config cache should have 2 entries (one per calling module), not 1")

	// Both should return valid results.
	assert.Equal(t, []string{"../app1"}, configA.Dependencies.Paths)
	assert.Equal(t, []string{"../app1"}, configB.Dependencies.Paths)
}
