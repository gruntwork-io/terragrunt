//go:build ssh

// We don't want contributors to have to install SSH keys to run these tests, so we skip
// them by default. Contributors need to opt in to run these tests by setting the
// build flag `ssh` when running the tests. This is done by adding the `-tags ssh` flag
// to the `go test` command. For example:
//
// go test -tags ssh ./...

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

func TestSSHCASGetterGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		url       string
		queryRef  string
		expectRef string
	}{
		{
			name:      "Basic URL without ref",
			url:       "github.com/gruntwork-io/terragrunt",
			expectRef: "",
		},
		{
			name:      "URL as SSH",
			url:       "git@github.com:gruntwork-io/terragrunt.git",
			expectRef: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			storePath := filepath.Join(tmpDir, "store")
			c, err := cas.New(cas.Options{StorePath: storePath})
			require.NoError(t, err)

			opts := &cas.CloneOptions{
				Branch: "main",
			}
			l := logger.CreateLogger()
			g := cas.NewCASGetter(l, c, opts)
			client := getter.Client{
				Getters: []getter.Getter{g},
			}

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
