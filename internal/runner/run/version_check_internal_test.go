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
		want         string
		versionFiles []string
	}{
		{
			name:         "version files slice is empty",
			workingDir:   "",
			versionFiles: nil,
			want:         "vf5EBFb5mAeonjrvR7hq6EKNHJo", // "no-version-files" scoped by working dir
		},
		{
			name:       "workdir contains version files",
			workingDir: "../../../test/fixtures/version-files-cache-key",
			versionFiles: []string{
				".terraform-version",
				".tool-versions",
			},
			want: "J0w7vl4Bn60byyu0y6A2NMmIMFI",
		},
		{
			name:       "workdir contains version files and we try to escape the working dir",
			workingDir: "../../../test/fixtures/version-files-cache-key",
			versionFiles: []string{
				".terraform-version",
				".tool-versions",
				"../../../dev/random",
			},
			want: "J0w7vl4Bn60byyu0y6A2NMmIMFI",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equalf(
				t,
				tt.want,
				computeVersionFilesCacheKey(tt.workingDir, tt.versionFiles),
				"computeVersionFilesCacheKey(%v, %v)",
				tt.workingDir,
				tt.versionFiles,
			)
		})
	}
}
