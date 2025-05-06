//go:build ssh

// We don't want contributors to have to install SSH keys to run these tests, so we skip
// them by default. Contributors need to opt in to run these tests by setting the
// build flag `ssh` when running the tests. This is done by adding the `-tags ssh` flag
// to the `go test` command. For example:
//
// go test -tags ssh ./...

package cas_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHCASGetterGet(t *testing.T) {
	t.Parallel()

	c, err := cas.New(cas.Options{})
	require.NoError(t, err)

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
			dstDir := filepath.Join(tmpDir, "repo")

			opts := &cas.CloneOptions{
				Branch: "main",
			}
			l := log.New()
			g := cas.NewCASGetter(&l, c, opts)
			client := getter.Client{
				Getters: []getter.Getter{g},
			}

			res, err := client.Get(
				context.TODO(),
				&getter.Request{
					Src: tt.url,
					Dst: dstDir,
				},
			)
			require.NoError(t, err)

			assert.Equal(t, dstDir, res.Dst)
		})
	}
}
