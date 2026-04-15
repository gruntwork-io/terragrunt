package cas_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/stretchr/testify/assert"
)

func TestSplitSourceDoubleSlash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		source     string
		wantBase   string
		wantSubdir string
	}{
		{
			name:       "with double slash",
			source:     "../..//modules/ec2-asg-service",
			wantBase:   "../..",
			wantSubdir: "modules/ec2-asg-service",
		},
		{
			name:       "without double slash",
			source:     "../../modules/ec2-asg-service",
			wantBase:   "../../modules/ec2-asg-service",
			wantSubdir: "",
		},
		{
			name:       "double slash at start",
			source:     "//modules/vpc",
			wantBase:   "",
			wantSubdir: "modules/vpc",
		},
		{
			name:       "only path",
			source:     "../units/service",
			wantBase:   "../units/service",
			wantSubdir: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base, subdir := cas.SplitSourceDoubleSlash(tt.source)
			assert.Equal(t, tt.wantBase, base)
			assert.Equal(t, tt.wantSubdir, subdir)
		})
	}
}

func TestDeterministicTreeHash(t *testing.T) {
	t.Parallel()

	// SHA-1 length refHash (40 chars) → produces SHA-1 output (40 chars)
	sha1Ref := "f39ea0ebf891c9954c89d07b73b487ff938ef08b"
	hash1 := cas.DeterministicTreeHash(sha1Ref, "stacks/ec2-asg-stateful-service")
	hash2 := cas.DeterministicTreeHash(sha1Ref, "stacks/ec2-asg-stateful-service")
	assert.Equal(t, hash1, hash2, "same inputs must produce the same hash")
	assert.Len(t, hash1, 40, "SHA-1 refHash should produce 40-char output")

	hash3 := cas.DeterministicTreeHash(sha1Ref, "stacks/different")
	assert.NotEqual(t, hash1, hash3)

	hash4 := cas.DeterministicTreeHash("0000000000000000000000000000000000000000", "stacks/ec2-asg-stateful-service")
	assert.NotEqual(t, hash1, hash4)

	// SHA-256 length refHash (64 chars) → produces SHA-256 output (64 chars)
	sha256Ref := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	hash5 := cas.DeterministicTreeHash(sha256Ref, "stacks/ec2-asg-stateful-service")
	assert.Len(t, hash5, 64, "SHA-256 refHash should produce 64-char output")
}
