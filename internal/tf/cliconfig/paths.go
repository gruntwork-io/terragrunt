package cliconfig

import (
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// EnvNameTFCLIConfigFile and EnvNameTerraformConfig name the CLI config file outright, in the order OpenTofu checks them.
const (
	EnvNameTFCLIConfigFile = "TF_CLI_CONFIG_FILE"
	EnvNameTerraformConfig = "TERRAFORM_CONFIG"
)

// windowsGOOS is the platform whose CLI config lives under %APPDATA%.
const windowsGOOS = "windows"

// UserConfigOverride returns the CLI config path an env var names, and the var that named it.
func UserConfigOverride(v *venv.Venv) (path, envName string) {
	for _, name := range []string{EnvNameTFCLIConfigFile, EnvNameTerraformConfig} {
		if override := v.Env[name]; override != "" {
			return override, name
		}
	}

	return "", ""
}

// UserConfigCandidates lists impl's default CLI-config file locations for the injected platform, most
// preferred first. Terraform reads only its own file; any other impl follows OpenTofu's search order.
func UserConfigCandidates(v *venv.Venv, impl tfimpl.Type) []string {
	home, err := v.Platform.UserHomeDir()
	if err != nil {
		home = ""
	}

	// On Windows tofu reads tofu.rc then terraform.rc under %APPDATA%; terraform reads only terraform.rc.
	if v.Platform.GOOS == windowsGOOS {
		appData := v.Env["APPDATA"]
		if appData == "" {
			return nil
		}

		if tfimpl.UsesTerraformCLIConfig(impl) {
			return []string{filepath.Join(appData, "terraform.rc")}
		}

		return []string{
			filepath.Join(appData, "tofu.rc"),
			filepath.Join(appData, "terraform.rc"),
		}
	}

	// terraform reads only ~/.terraformrc and never falls back to XDG.
	if tfimpl.UsesTerraformCLIConfig(impl) {
		if home == "" {
			return nil
		}

		return []string{filepath.Join(home, ".terraformrc")}
	}

	var paths []string

	if home != "" {
		paths = append(paths, filepath.Join(home, ".tofurc"), filepath.Join(home, ".terraformrc"))
	}

	// tofu falls back to XDG only when XDG_CONFIG_HOME is set, and never synthesizes ~/.config.
	if configDir := v.Env["XDG_CONFIG_HOME"]; configDir != "" {
		paths = append(paths, filepath.Join(configDir, "opentofu", "tofurc"))
	}

	return paths
}

// UserConfigDir resolves impl's CLI config directory, whose *.tfrc fragments extend the CLI config.
func UserConfigDir(v *venv.Venv, impl tfimpl.Type) (string, error) {
	home, err := v.Platform.UserHomeDir()
	if err != nil {
		home = ""
	}

	if v.Platform.GOOS == windowsGOOS {
		if appData := v.Env["APPDATA"]; appData != "" {
			return filepath.Join(appData, "terraform.d"), nil
		}

		return "", nil
	}

	// terraform always uses ~/.terraform.d and never falls back to XDG.
	if tfimpl.UsesTerraformCLIConfig(impl) {
		if home == "" {
			return "", nil
		}

		return filepath.Join(home, ".terraform.d"), nil
	}

	if home != "" {
		legacy := filepath.Join(home, ".terraform.d")

		exists, err := vfs.FileExists(v.FS, legacy)
		if err != nil {
			return "", fmt.Errorf("reading OpenTofu CLI config directory %s: %w", legacy, err)
		}

		if exists {
			return legacy, nil
		}
	}

	// tofu falls back to XDG only when XDG_CONFIG_HOME is set, and never synthesizes ~/.config.
	if xdg := v.Env["XDG_CONFIG_HOME"]; xdg != "" {
		return filepath.Join(xdg, "opentofu"), nil
	}

	if home == "" {
		return "", nil
	}

	return filepath.Join(home, ".terraform.d"), nil
}
