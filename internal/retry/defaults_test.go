package retry_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/retry"
	"github.com/stretchr/testify/assert"
)

func TestDefaultRetryableErrorsMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		errMsg    string
		wantMatch bool
	}{
		// OpenTofu provider resolution errors (the CI failures that prompted this change)
		{
			name:      "opentofu context deadline on provider resolve",
			errMsg:    "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/null: could not connect to registry.opentofu.org: failed to request discovery document: Get \"https://registry.opentofu.org/.well-known/terraform.json\": context deadline exceeded",
			wantMatch: true,
		},
		{
			name:      "opentofu TLS handshake timeout on provider resolve",
			errMsg:    "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/aws: could not connect to registry.opentofu.org: TLS handshake timeout",
			wantMatch: true,
		},
		{
			name:      "opentofu tcp timeout on provider resolve",
			errMsg:    "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/null: could not connect to registry.opentofu.org: tcp connection timeout",
			wantMatch: true,
		},
		{
			name:      "opentofu tcp connection reset on provider resolve",
			errMsg:    "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/null: could not connect to registry.opentofu.org: tcp: connection reset by peer",
			wantMatch: true,
		},
		{
			name:      "opentofu failed to query available provider packages",
			errMsg:    "Error: Failed to query available provider packages\nCould not retrieve the list of available versions for provider hashicorp/null: could not connect to registry.opentofu.org: context deadline exceeded",
			wantMatch: true,
		},
		{
			name:      "opentofu discovery document deadline",
			errMsg:    "failed to request discovery document: Get \"https://registry.opentofu.org/.well-known/terraform.json\": context deadline exceeded",
			wantMatch: true,
		},

		// Terraform provider installation errors (existing behavior preserved)
		{
			name:      "terraform context deadline on provider query",
			errMsg:    "Error: Failed to install provider\ncould not query provider registry for registry.terraform.io/hashicorp/null: context deadline exceeded",
			wantMatch: true,
		},
		{
			name:      "terraform TLS handshake timeout installing provider",
			errMsg:    "Error installing provider \"hashicorp/aws\": TLS handshake timeout",
			wantMatch: true,
		},
		{
			name:      "terraform tcp timeout installing provider",
			errMsg:    "Error installing provider \"hashicorp/null\": tcp connection timeout",
			wantMatch: true,
		},
		{
			name:      "terraform tcp connection reset installing provider",
			errMsg:    "Error installing provider \"hashicorp/null\": tcp: connection reset by peer",
			wantMatch: true,
		},

		// Registry connection errors
		{
			name:      "registry context deadline exceeded",
			errMsg:    "could not connect to registry.opentofu.org: context deadline exceeded",
			wantMatch: true,
		},
		{
			name:      "terraform registry context deadline",
			errMsg:    "could not query provider registry for registry.terraform.io/hashicorp/template: context deadline exceeded",
			wantMatch: true,
		},

		// Other existing retryable errors
		{
			name:      "state load tcp timeout",
			errMsg:    "Failed to load state: tcp connection timeout",
			wantMatch: true,
		},
		{
			name:      "backend TLS handshake timeout",
			errMsg:    "Failed to load backend: TLS handshake timeout",
			wantMatch: true,
		},
		{
			name:      "client timeout awaiting headers",
			errMsg:    "Client.Timeout exceeded while awaiting headers",
			wantMatch: true,
		},
		{
			name:      "module download 429",
			errMsg:    "Could not download module \"foo\": The requested URL returned error: 429",
			wantMatch: true,
		},
		{
			name:      "generic provider context deadline",
			errMsg:    "provider hashicorp/null: context deadline exceeded",
			wantMatch: true,
		},
		{
			name:      "generic registry context deadline",
			errMsg:    "registry.terraform.io: context deadline exceeded",
			wantMatch: true,
		},

		// Permanent errors that must NOT match
		{
			name:      "provider not found is permanent",
			errMsg:    "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/nonexistent: provider registry registry.opentofu.org does not have a provider named hashicorp/nonexistent",
			wantMatch: false,
		},
		{
			name:      "version constraint mismatch is permanent",
			errMsg:    "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/aws: no available releases match the given constraints >= 99.0.0",
			wantMatch: false,
		},
		{
			name:      "syntax error is permanent",
			errMsg:    "Error: Invalid provider configuration\nProvider \"hashicorp/aws\" requires explicit configuration",
			wantMatch: false,
		},
		{
			name:      "unrelated context deadline not matched",
			errMsg:    "Error: context deadline exceeded while waiting for user input",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			matched := false

			for _, re := range retry.DefaultRetryableRegexps {
				if re.MatchString(tt.errMsg) {
					matched = true
					break
				}
			}

			assert.Equal(t, tt.wantMatch, matched, "error message: %q", tt.errMsg)
		})
	}
}
