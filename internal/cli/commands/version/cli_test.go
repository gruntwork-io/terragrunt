package version_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/version"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCommandPrintsTheAppVersion pins that `terragrunt version` prints the app version and exits cleanly.
func TestNewCommandPrintsTheAppVersion(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)

	app := clihelper.NewApp(map[string]string{})
	app.Writer = out
	app.Version = "v1.2.3"
	app.CustomAppVersionTemplate = "terragrunt version {{.Version}}"
	app.ExitErrHandler = func(_ *clihelper.Context, err error) error { return err }

	cmd := version.NewCommand()
	assert.Equal(t, version.CommandName, cmd.Name)

	require.NoError(t, cmd.Run(t.Context(), clihelper.NewAppContext(app, nil), nil))
	assert.Equal(t, "terragrunt version v1.2.3", out.String())
}
