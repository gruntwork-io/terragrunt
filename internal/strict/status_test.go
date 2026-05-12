package strict_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/stretchr/testify/assert"
)

const (
	ansiGreen  = "\033[0;32m"
	ansiYellow = "\033[0;33m"
	ansiReset  = "\033[0m"
)

func TestStatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		want   string
		status strict.Status
	}{
		{name: "active", want: "Active", status: strict.ActiveStatus},
		{name: "completed", want: "Completed", status: strict.CompletedStatus},
		{name: "unknown", want: "unknown", status: strict.Status(99)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.status.String())
		})
	}
}

func TestStatusStringWithANSIColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		want   string
		status strict.Status
	}{
		{name: "active green", want: ansiGreen + "Active" + ansiReset, status: strict.ActiveStatus},
		{name: "completed yellow", want: ansiYellow + "Completed" + ansiReset, status: strict.CompletedStatus},
		{name: "unknown bare", want: "unknown", status: strict.Status(99)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.status.StringWithANSIColor())
		})
	}
}
