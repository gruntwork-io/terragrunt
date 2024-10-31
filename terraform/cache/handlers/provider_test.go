package handlers_test

import (
	"errors"
	"syscall"
	"testing"

	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"

	"github.com/stretchr/testify/assert"
)

func TestIsOfflineError(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		err      error
		expected bool
		desc     string
	}{
		{syscall.ECONNREFUSED, true, "connection refused"},
		{syscall.ECONNRESET, true, "connection reset by peer"},
		{syscall.ECONNABORTED, true, "connection aborted"},
		{syscall.ENETUNREACH, true, "network is unreachable"},
		{errors.New("get \"https://registry.terraform.io/.well-known/terraform.json\": dial tcp: lookup registry.terraform.io on 185.12.64.1:53: dial udp 185.12.64.1:53: connect: network is unreachable"), true, "network is unreachable"},
		{errors.New("get \"https://registry.terraform.io/.well-known/terraform.json\": read tcp 10.10.230.10:58328->10.245.10.15:443: read: connection reset by peer"), true, "network is unreachable"},
		{errors.New("random error"), false, "a random error that should not be offline"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			result := handlers.IsOfflineError(tc.err)
			assert.Equal(t, tc.expected, result, "Expected result for %v is %v", tc.desc, tc.expected)
		})
	}
}
