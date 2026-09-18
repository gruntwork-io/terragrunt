package discovery_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/shell/split"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/require"
)

func TestNewForDiscoveryCommand_QueueConstructAs(t *testing.T) {
	t.Parallel()

	newForDiscoveryCommand := func(t *testing.T, queueConstructAs string) (*discovery.Discovery, error) {
		t.Helper()

		v := venvtest.New()

		return discovery.NewForDiscoveryCommand(
			logger.CreateLogger(),
			v.FS,
			&discovery.DiscoveryCommandOptions{
				WorkingDir:       venvtest.Root("/repo"),
				QueueConstructAs: queueConstructAs,
			},
		)
	}

	emptyCases := []struct {
		name             string
		queueConstructAs string
	}{
		{name: "whitespace", queueConstructAs: "   "},
		{name: "tab", queueConstructAs: "\t"},
		{name: "double quoted empty string", queueConstructAs: `""`},
		{name: "single quoted empty string", queueConstructAs: "''"},
	}

	for _, tc := range emptyCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := newForDiscoveryCommand(t, tc.queueConstructAs)
			require.ErrorAs(t, err, &discovery.EmptyQueueConstructAsError{})
		})
	}

	operatorCases := []struct {
		name             string
		queueConstructAs string
	}{
		{name: "semicolon", queueConstructAs: ";"},
		{name: "pipe", queueConstructAs: "|"},
		{name: "logical and", queueConstructAs: "&&"},
		{name: "redirect", queueConstructAs: ">"},
		{name: "stderr redirect", queueConstructAs: "2>&1"},
		{name: "command after separator", queueConstructAs: "; plan"},
		{name: "separator after command", queueConstructAs: "plan; apply"},
	}

	for _, tc := range operatorCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := newForDiscoveryCommand(t, tc.queueConstructAs)
			require.ErrorIs(t, err, split.ErrShellOperator)
		})
	}

	t.Run("command with arguments", func(t *testing.T) {
		t.Parallel()

		d, err := newForDiscoveryCommand(t, "apply -destroy")
		require.NoError(t, err)
		require.NotNil(t, d)
	})

	t.Run("unbalanced quote", func(t *testing.T) {
		t.Parallel()

		_, err := newForDiscoveryCommand(t, "plan '")
		require.Error(t, err)
		require.NotErrorAs(t, err, &discovery.EmptyQueueConstructAsError{})
	})
}
