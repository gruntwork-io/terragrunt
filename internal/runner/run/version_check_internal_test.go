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
			want:         "r01AJjVD7VSXCQk1ORuh_no_NRY", // "no-version-files"
		},
		{
			name:       "workdir contains version files",
			workingDir: "../../../test/fixtures/version-files-cache-key",
			versionFiles: []string{
				".terraform-version",
				".tool-versions",
			},
			want: "XBE-VO9pOnQjPQDmLQCvSCdckSQ",
		},
		{
			name:       "workdir contains version files and we try to escape the working dir",
			workingDir: "../../../test/fixtures/version-files-cache-key",
			versionFiles: []string{
				".terraform-version",
				".tool-versions",
				"../../../dev/random",
			},
			want: "XBE-VO9pOnQjPQDmLQCvSCdckSQ",
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
