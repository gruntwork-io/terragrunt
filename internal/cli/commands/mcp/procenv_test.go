package mcp_test

import (
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/stretchr/testify/assert"
)

const (
	isolatedHome = "/empty/home"
	isolatedBin  = "/empty/bin"
)

// serverEnv returns an environment shaped like a developer's shell, with
// credentials for each cloud SDK the server links.
func serverEnv() map[string]string {
	return map[string]string{
		"PATH":                           "/usr/local/bin:/usr/bin:/bin",
		"HOME":                           "/home/dev",
		"TMPDIR":                         "/var/tmp/dev",
		"AWS_ACCESS_KEY_ID":              "AKIAEXAMPLE",
		"AWS_SHARED_CREDENTIALS_FILE":    "/srv/aws/credentials",
		"GOOGLE_APPLICATION_CREDENTIALS": "/srv/gcp/adc.json",
		"AZURE_CLIENT_SECRET":            "secret",
		"XDG_CONFIG_HOME":                "/home/dev/.config",
	}
}

// TestIsolatedEnvWithNothingGrantedKeepsOnlyTheTempDir pins that a server
// granted nothing leaves the SDKs no credential variable, no home directory,
// and no PATH to spawn from.
func TestIsolatedEnvWithNothingGrantedKeepsOnlyTheTempDir(t *testing.T) {
	t.Parallel()

	got := tgmcp.IsolatedEnv(serverEnv(), tgmcp.Capabilities{}, isolatedHome, isolatedBin)

	assert.Equal(t, map[string]string{
		"PATH":         isolatedBin,
		"TMPDIR":       "/var/tmp/dev",
		"HOME":         isolatedHome,
		"USERPROFILE":  isolatedHome,
		"APPDATA":      isolatedHome,
		"LOCALAPPDATA": isolatedHome,
	}, got)
}

// TestIsolatedEnvUnderExecKeepsPath pins that granting exec alone keeps the
// PATH Terragrunt finds tofu and git on, while the credentials still go.
func TestIsolatedEnvUnderExecKeepsPath(t *testing.T) {
	t.Parallel()

	granted := tgmcp.Capabilities{tgmcp.CapabilityExec: true}

	got := tgmcp.IsolatedEnv(serverEnv(), granted, isolatedHome, isolatedBin)

	assert.Equal(t, "/usr/local/bin:/usr/bin:/bin", got["PATH"])
	assert.Equal(t, isolatedHome, got["HOME"])
	assert.NotContains(t, got, "AWS_ACCESS_KEY_ID")
}

// TestIsolatedEnvUnderEnvKeepsEverythingButPath pins that granting env alone
// leaves the SDKs the server's credentials and home, while PATH still points
// at the empty directory.
func TestIsolatedEnvUnderEnvKeepsEverythingButPath(t *testing.T) {
	t.Parallel()

	granted := tgmcp.Capabilities{tgmcp.CapabilityEnv: true}

	want := serverEnv()
	want["PATH"] = isolatedBin

	assert.Equal(t, want, tgmcp.IsolatedEnv(serverEnv(), granted, isolatedHome, isolatedBin))
}

func TestIsolatedEnvUnderExecAndEnvChangesNothing(t *testing.T) {
	t.Parallel()

	granted := tgmcp.Capabilities{tgmcp.CapabilityExec: true, tgmcp.CapabilityEnv: true}

	assert.Equal(t, serverEnv(), tgmcp.IsolatedEnv(serverEnv(), granted, isolatedHome, isolatedBin))
}

// TestIsolatedEnvMatchesWindowsNamesIgnoringCase pins that Windows' Path and
// SystemRoot spellings are kept or replaced like PATH, so the real Path never
// survives beside the replacement.
func TestIsolatedEnvMatchesWindowsNamesIgnoringCase(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"Path":       `C:\Windows\system32`,
		"SystemRoot": `C:\Windows`,
		"TEMP":       `C:\Users\dev\AppData\Local\Temp`,
	}

	t.Run("replaced", func(t *testing.T) {
		t.Parallel()

		got := tgmcp.IsolatedEnv(env, tgmcp.Capabilities{tgmcp.CapabilityEnv: true}, isolatedHome, isolatedBin)

		assert.Equal(t, map[string]string{
			"PATH":       isolatedBin,
			"SystemRoot": `C:\Windows`,
			"TEMP":       `C:\Users\dev\AppData\Local\Temp`,
		}, got)
	})

	t.Run("kept", func(t *testing.T) {
		t.Parallel()

		got := tgmcp.IsolatedEnv(env, tgmcp.Capabilities{tgmcp.CapabilityExec: true}, isolatedHome, isolatedBin)

		assert.Equal(t, `C:\Windows\system32`, got["Path"])
		assert.Equal(t, `C:\Windows`, got["SystemRoot"])
		assert.Equal(t, `C:\Users\dev\AppData\Local\Temp`, got["TEMP"])
	})
}
