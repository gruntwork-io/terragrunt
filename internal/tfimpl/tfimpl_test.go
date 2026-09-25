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

func TestDefaultRegistryDomain(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		env  map[string]string
		impl tfimpl.Type
		want string
	}{
		{
			name: "opentofu",
			env:  map[string]string{},
			impl: tfimpl.OpenTofu,
			want: "registry.opentofu.org",
		},
		{
			name: "terraform",
			env:  map[string]string{},
			impl: tfimpl.Terraform,
			want: "registry.terraform.io",
		},
		{
			name: "unknown",
			env:  map[string]string{},
			impl: tfimpl.Unknown,
			want: "registry.opentofu.org",
		},
		{
			name: "unset",
			env:  map[string]string{},
			impl: "",
			want: "registry.opentofu.org",
		},
		{
			name: "env override",
			env:  map[string]string{"TG_TF_DEFAULT_REGISTRY_HOST": "registry.example.com"},
			impl: tfimpl.Terraform,
			want: "registry.example.com",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tfimpl.DefaultRegistryDomain(tc.env, tc.impl))
		})
	}
}
