package retry_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
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
			name: "opentofu context deadline on provider resolve",
			errMsg: "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/null:" +
				" could not connect to registry.opentofu.org: failed to request discovery document:" +
				` Get "https://registry.opentofu.org/.well-known/terraform.json": context deadline exceeded`,
			wantMatch: true,
		},
		{
			name: "opentofu TLS handshake timeout on provider resolve",
			errMsg: "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/aws:" +
				" could not connect to registry.opentofu.org: TLS handshake timeout",
			wantMatch: true,
		},
		{
			name: "opentofu tcp timeout on provider resolve",
			errMsg: "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/null:" +
				" could not connect to registry.opentofu.org: tcp connection timeout",
			wantMatch: true,
		},
		{
			name: "opentofu tcp connection reset on provider resolve",
			errMsg: "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/null:" +
				" could not connect to registry.opentofu.org: tcp: connection reset by peer",
			wantMatch: true,
		},
		{
			name: "opentofu failed to query available provider packages",
			errMsg: "Error: Failed to query available provider packages\n" +
				"Could not retrieve the list of available versions for provider hashicorp/null:" +
				" could not connect to registry.opentofu.org: context deadline exceeded",
			wantMatch: true,
		},
		{
			name: "opentofu discovery document deadline",
			errMsg: `failed to request discovery document: Get "https://registry.opentofu.org/.well-known/terraform.json":` +
				" context deadline exceeded",
			wantMatch: true,
		},

		// Terraform provider installation errors (existing behavior preserved)
		{
			name: "terraform context deadline on provider query",
			errMsg: "Error: Failed to install provider\ncould not query provider registry for" +
				" registry.terraform.io/hashicorp/null: context deadline exceeded",
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
			name: "terraform registry context deadline",
			errMsg: "could not query provider registry for" +
				" registry.terraform.io/hashicorp/template: context deadline exceeded",
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

		// Parse-phase sops_decrypt_file transient failures (issue #6755, retry-parse-errors experiment)
		{
			name: "sops decrypt i/o timeout against KMS",
			errMsg: `Call to function "sops_decrypt_file" failed: error decrypting key: failed to decrypt sops` +
				" data key with GCP KMS key: rpc error: code = Unauthenticated desc = transport: per-RPC creds" +
				` failed due to error: credentials: invalid response when retrieving subject token: Get "https://x":` +
				" dial tcp 20.85.130.105:443: i/o timeout",
			wantMatch: true,
		},
		{
			name:      "sops decrypt 503 from KMS",
			errMsg:    `Call to function "sops_decrypt_file" failed: error decrypting key: 503 Service Unavailable`,
			wantMatch: true,
		},
		{
			name:      "sops decrypt 429 from token endpoint",
			errMsg:    `Call to function "sops_decrypt_file" failed: error decrypting key: 429 Too Many Requests`,
			wantMatch: true,
		},
		{
			name: "sops decrypt i/o timeout after error cleaning",
			errMsg: errorconfig.ExtractErrorMessage(errors.New(
				`/repo/app/terragrunt.hcl:3,12-30: Error in function call; Call to function "sops_decrypt_file"` +
					" failed: error decrypting key: dial tcp 20.85.130.105:443: i/o timeout.")),
			wantMatch: true,
		},
		{
			name: "sops decrypt client timeout awaiting headers",
			errMsg: `Call to function "sops_decrypt_file" failed: error decrypting key:` +
				" net/http: request canceled (Client.Timeout exceeded while awaiting headers)",
			wantMatch: true,
		},
		{
			name: "sops decrypt with wrong key is permanent",
			errMsg: `Call to function "sops_decrypt_file" failed:` +
				" Error getting data key: 0 successful groups required, got 0",
			wantMatch: false,
		},
		{
			name: "sops decrypt rpc unavailable from KMS",
			errMsg: `Call to function "sops_decrypt_file" failed: error decrypting key:` +
				" rpc error: code = Unavailable desc = the service is currently unavailable",
			wantMatch: true,
		},
		{
			name: "sops decrypt numeric-only 429 from oauth2 token endpoint",
			errMsg: `Call to function "sops_decrypt_file" failed: error decrypting key:` +
				" oauth2/google/externalaccount: status code 429:",
			wantMatch: true,
		},
		{
			name: "sops decrypt numeric-only 503 from AWS KMS transport",
			errMsg: `Call to function "sops_decrypt_file" failed: error decrypting key:` +
				" operation error KMS: Decrypt, https response error StatusCode: 503, RequestID: abc",
			wantMatch: true,
		},
		{
			name: "sops decrypt context deadline exceeded",
			errMsg: `Call to function "sops_decrypt_file" failed: error decrypting key:` +
				" context deadline exceeded",
			wantMatch: true,
		},
		{
			name: "sops decrypt connection reset by peer",
			errMsg: `Call to function "sops_decrypt_file" failed: error decrypting key:` +
				" read tcp 10.0.0.5:54322: connection reset by peer",
			wantMatch: true,
		},
		{
			name: "sops decrypt TLS handshake timeout",
			errMsg: `Call to function "sops_decrypt_file" failed: error decrypting key:` +
				` Post "https://cloudkms.googleapis.com/v1/key:decrypt": TLS handshake timeout`,
			wantMatch: true,
		},
		{
			name: "sops decrypt access denied with 503 inside an account id is permanent",
			errMsg: `Call to function "sops_decrypt_file" failed: AccessDenied: user` +
				" arn:aws:iam::503212345678:role/deploy is not authorized to perform kms:Decrypt",
			wantMatch: false,
		},
		{
			name: "sops decrypt missing file under a 503 directory is permanent",
			errMsg: `Call to function "sops_decrypt_file" failed:` +
				" open secrets/503/missing.json: no such file or directory",
			wantMatch: false,
		},
		{
			name: "sops decrypt missing file under a 429 directory is permanent",
			errMsg: `Call to function "sops_decrypt_file" failed:` +
				" open secrets/429/missing.json: no such file or directory",
			wantMatch: false,
		},
		{
			name: "sops decrypt missing file under an Unavailable directory is permanent",
			errMsg: `Call to function "sops_decrypt_file" failed:` +
				" open secrets/Unavailable/missing.json: no such file or directory",
			wantMatch: false,
		},

		// Permanent errors that must NOT match
		{
			name: "provider not found is permanent",
			errMsg: "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/nonexistent:" +
				" provider registry registry.opentofu.org does not have a provider named hashicorp/nonexistent",
			wantMatch: false,
		},
		{
			name: "version constraint mismatch is permanent",
			errMsg: "Error: Failed to resolve provider packages\nCould not resolve provider hashicorp/aws:" +
				" no available releases match the given constraints >= 99.0.0",
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
