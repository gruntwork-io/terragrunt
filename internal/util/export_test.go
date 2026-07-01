package util

func NewFileManifestWithEntryCapForTest(manifestFolder string, manifestFile string, maxEntries int) *fileManifest {
	manifest := NewFileManifest(manifestFolder, manifestFile)
	manifest.maxEntries = maxEntries

	return manifest
}
