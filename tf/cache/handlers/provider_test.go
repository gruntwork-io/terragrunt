package handlers_test

import (
	"errors"
	"syscall"
	"testing"

	"github.com/gruntwork-io/terragrunt/tf/cache/handlers"

	"github.com/stretchr/testify/assert"
)

func TestIsOfflineError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		err      error
		desc     string
		expected bool
	}{
		{err: syscall.ECONNREFUSED, desc: "connection refused", expected: true},
		{err: syscall.ECONNRESET, desc: "connection reset by peer", expected: true},
		{err: syscall.ECONNABORTED, desc: "connection aborted", expected: true},
		{err: syscall.ENETUNREACH, desc: "network is unreachable", expected: true},
		{err: errors.New("get \"https://registry.terraform.io/.well-known/terraform.json\": dial tcp: lookup registry.terraform.io on 185.12.64.1:53: dial udp 185.12.64.1:53: connect: network is unreachable"), desc: "network is unreachable", expected: true},
		{err: errors.New("get \"https://registry.terraform.io/.well-known/terraform.json\": read tcp 10.10.230.10:58328->10.245.10.15:443: read: connection reset by peer"), desc: "network is unreachable", expected: true},
		{err: errors.New("random error"), desc: "a random error that should not be offline", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			result := handlers.IsOfflineError(tc.err)
			assert.Equal(t, tc.expected, result, "Expected result for %v is %v", tc.desc, tc.expected)
		})
	}
}
