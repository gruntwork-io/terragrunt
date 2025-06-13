// Tests specific to race conditions are verified here

package cas_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCASGetterGetWithRacing(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "store")

	c, err := cas.New(cas.Options{
		StorePath: storePath,
	})
	require.NoError(t, err)

	opts := &cas.CloneOptions{
		Branch: "main",
	}

	l := logger.CreateLogger()

	g := cas.NewCASGetter(l, c, opts)
	client := getter.Client{
		Getters: []getter.Getter{g},
	}

	tests := []struct {
		name      string
		url       string
		queryRef  string
		expectRef string
	}{
		{
			name:      "URL with ref parameter",
			url:       "github.com/gruntwork-io/terragrunt?ref=v0.75.0",
			expectRef: "v0.75.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()

			res, err := client.Get(
				t.Context(),
				&getter.Request{
					Src: tt.url,
					Dst: tmpDir,
				},
			)
			require.NoError(t, err)

			assert.Equal(t, tmpDir, res.Dst)
		})
	}
}
