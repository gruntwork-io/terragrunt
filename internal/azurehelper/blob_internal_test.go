//go:build azure

package azurehelper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWithPrefix exercises the variadic-option plumbing without needing a
// live Azure storage account. Verifies that WithPrefix populates the private
// listBlobsOptions.prefix field that ListBlobs reads into
// azblobcontainer.ListBlobsFlatOptions.Prefix.
func TestWithPrefix(t *testing.T) {
	t.Parallel()

	o := &listBlobsOptions{}
	WithPrefix("state/")(o)
	assert.Equal(t, "state/", o.prefix)

	// Zero options leave prefix empty (ListBlobs treats this as "no filter").
	empty := &listBlobsOptions{}
	assert.Empty(t, empty.prefix)
}
