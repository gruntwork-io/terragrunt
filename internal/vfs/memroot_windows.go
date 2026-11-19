//go:build windows

package vfs

// MemRoot is an absolute path to build a [NewMemMapFS] tree under, for tests that need an absolute root but no
// directory on disk.
const MemRoot = `C:\mem`
