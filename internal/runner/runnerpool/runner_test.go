package runnerpool_test

import (
	"testing"
)

// TestDiscoveryResolverMatchesLegacyPaths was removed because UnitResolver has been moved to discovery package.
// Discovery now handles unit resolution directly when WithTerragruntOptions() is called.
// The functionality previously tested here is now covered by discovery package tests.
func TestDiscoveryResolverMatchesLegacyPaths(t *testing.T) {
	t.Parallel()
	// Test removed - UnitResolver logic moved to discovery package
	t.Skip("UnitResolver has been moved to discovery package")
}
