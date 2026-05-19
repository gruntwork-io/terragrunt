package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_computeVersionFilesCacheKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		workingDir   string
		tfPath       string
		want         string
		versionFiles []string
	}{
		{
			name:         "no version files, tofu binary",
			workingDir:   "",
			tfPath:       "tofu",
			versionFiles: nil,
			want:         "H2PcpB8dh-BE5Dz-LiSD1hykSfk", // "tofu|no-version-files"
		},
		{
			// Same inputs as the previous case except for the binary. The key
			// must differ. That is the property issue #6147 turned on.
			name:         "no version files, terraform binary",
			workingDir:   "",
			tfPath:       "terraform",
			versionFiles: nil,
			want:         "rdE9RkSvu7WDQly2KzL2HxpcB3Q", // "terraform|no-version-files"
		},
		{
			name:       "workdir contains version files",
			workingDir: "../../../test/fixtures/version-files-cache-key",
			tfPath:     "tofu",
			versionFiles: []string{
				".terraform-version",
				".tool-versions",
			},
			want: "trAjGOdUv3IcX1lU50dck_mqlUs",
		},
		{
			// SanitizePath strips the path-traversal entry before hashing, so
			// this case must collapse to the same key as the previous one.
			name:       "workdir contains version files and we try to escape the working dir",
			workingDir: "../../../test/fixtures/version-files-cache-key",
			tfPath:     "tofu",
			versionFiles: []string{
				".terraform-version",
				".tool-versions",
				"../../../dev/random",
			},
			want: "trAjGOdUv3IcX1lU50dck_mqlUs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equalf(
				t,
				tt.want,
				computeVersionFilesCacheKey(tt.workingDir, tt.versionFiles, tt.tfPath),
				"computeVersionFilesCacheKey(%v, %v, %v)",
				tt.workingDir,
				tt.versionFiles,
				tt.tfPath,
			)
		})
	}
}
