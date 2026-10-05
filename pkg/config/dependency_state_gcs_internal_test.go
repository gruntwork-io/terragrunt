package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGCSEffectiveImpersonateServiceAccount(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT": "backend@example.com",
		"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT":         "provider@example.com",
	}

	testCases := []struct {
		env      map[string]string
		name     string
		expected string
		settings gcsDirectStateReadSettings
	}{
		{
			name: "configured account wins over the environment",
			env:  env,
			settings: gcsDirectStateReadSettings{
				impersonateServiceAccount:           "configured@example.com",
				impersonateServiceAccountConfigured: true,
			},
			expected: "configured@example.com",
		},
		{
			name:     "configured empty account suppresses the environment",
			env:      env,
			settings: gcsDirectStateReadSettings{impersonateServiceAccountConfigured: true},
			expected: "",
		},
		{
			name:     "backend variable wins over the provider variable",
			env:      env,
			expected: "backend@example.com",
		},
		{
			name:     "provider variable is the last fallback",
			env:      map[string]string{"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT": "provider@example.com"},
			expected: "provider@example.com",
		},
		{
			name:     "nothing configured means no impersonation",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, gcsEffectiveImpersonateServiceAccount(tc.env, &tc.settings))
		})
	}
}
