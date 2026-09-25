package tfimpl_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/stretchr/testify/assert"
)

func TestTypeDisplayName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		impl tfimpl.Type
		want string
	}{
		{impl: tfimpl.OpenTofu, want: "OpenTofu"},
		{impl: tfimpl.Terraform, want: "Terraform"},
		{impl: tfimpl.Unknown, want: "OpenTofu/Terraform"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.impl), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.impl.DisplayName())
		})
	}
}
