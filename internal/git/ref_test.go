package git_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref  string
		want string
	}{
		{ref: "main", want: "main"},
		{ref: "HEAD~1", want: "HEAD_1"},
		{ref: "feature/my-branch_2", want: "feature_my-branch_2"},
		{ref: "..", want: "__"},
		{ref: "v1.2.3", want: "v1_2_3"},
		{ref: "héllo", want: "h_llo"},
	}

	for _, tc := range tests {
		t.Run(tc.ref, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, git.SanitizeRef(tc.ref))
		})
	}
}
