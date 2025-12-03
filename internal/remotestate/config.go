package remotestate

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/zclconf/go-cty/cty"
)

var (
	ErrRemoteBackendMissing             = errors.New("the remote_state.backend field cannot be empty")
	ErrGenerateCalledWithNoGenerateAttr = errors.New("generate code routine called when no generate attribute is configured")
)

// ConfigGenerate is code gen configuration for Terraform remote state.
type ConfigGenerate struct {
	Path     string `cty:"path" mapstructure:"path"`
	IfExists string `cty:"if_exists" mapstructure:"if_exists"`
}

// Config is the configuration for Terraform remote state.
// NOTE: If any attributes are added here, be sure to add it to `ConfigFile` struct.
type Config struct {
	BackendConfig                 backend.Config  `mapstructure:"config" json:"Config"`
	Generate                      *ConfigGenerate `mapstructure:"generate" json:"Generate"`
	Encryption                    map[string]any  `mapstructure:"encryption" json:"Encryption"`
	BackendName                   string          `mapstructure:"backend" json:"Backend"`
	DisableInit                   bool            `mapstructure:"disable_init" json:"DisableInit"`
	DisableDependencyOptimization bool            `mapstructure:"disable_dependency_optimization" json:"DisableDependencyOptimization"`
}

func (cfg *Config) String() string {
	return fmt.Sprintf(
		"RemoteState{Backend = %v, DisableInit = %v, DisableDependencyOptimization = %v, Generate = %v, Config = %v, Encryption = %v}",
		cfg.BackendName,
		cfg.DisableInit,
		cfg.DisableDependencyOptimization,
		cfg.Generate,
		cfg.BackendConfig,
		cfg.Encryption,
	)
}

// Validate validates that the remote state is configured correctly.
func (cfg *Config) Validate() error {
	if cfg.BackendName == "" {
		return errors.New(ErrRemoteBackendMissing)
	}

	return nil
}

// GenerateOpenTofuCode generates the OpenTofu/Terraform code for configuring remote state backend.
func (cfg *Config) GenerateOpenTofuCode(l log.Logger, opts *options.TerragruntOptions, backendConfig map[string]any) error {
	if cfg.Generate == nil {
		return errors.New(ErrGenerateCalledWithNoGenerateAttr)
	}

	switch {
	case cfg.Encryption == nil:
		l.Debug("No encryption block in remote_state config")
	case len(cfg.Encryption) == 0:
		l.Debug("Empty encryption block in remote_state config")
	default:
		_, ok := cfg.Encryption[codegen.EncryptionKeyProviderKey].(string)
		if !ok {
			return errors.New("key_provider not found in encryption config")
		}
	}

	// Convert the IfExists setting to the internal enum representation before calling generate.
	ifExistsEnum, err := codegen.GenerateConfigExistsFromString(cfg.Generate.IfExists)
	if err != nil {
		return err
	}

	configBytes, err := codegen.RemoteStateConfigToTerraformCode(cfg.BackendName, backendConfig, cfg.Encryption)
	if err != nil {
		return err
	}

	codegenConfig := codegen.GenerateConfig{
		Path:          cfg.Generate.Path,
		IfExists:      ifExistsEnum,
		IfExistsStr:   cfg.Generate.IfExists,
		Contents:      string(configBytes),
		CommentPrefix: codegen.DefaultCommentPrefix,
	}

	return codegen.WriteToFile(l, opts, opts.WorkingDir, codegenConfig)
}

type ConfigFileGenerate struct {
	// We use cty instead of hcl, since we are using this type to convert an attr and not a block.
	Path     string `cty:"path"`
	IfExists string `cty:"if_exists"`
}

// ConfigFile is configuration for Terraform remote state as parsed from a terragrunt.hcl config file.
type ConfigFile struct {
	BackendConfig                 cty.Value           `hcl:"config,attr"`
	DisableInit                   *bool               `hcl:"disable_init,attr"`
	DisableDependencyOptimization *bool               `hcl:"disable_dependency_optimization,attr"`
	Generate                      *ConfigFileGenerate `hcl:"generate,attr"`
	Encryption                    *cty.Value          `hcl:"encryption,attr"`
	BackendName                   string              `hcl:"backend,attr"`
}

func (cfgFile *ConfigFile) String() string {
	return fmt.Sprintf("ConfigFile{Backend = %v, Config = %v}",
		cfgFile.BackendName,
		cfgFile.BackendConfig,
	)
}

// Config converts the parsed config file remote state struct to the internal representation struct of remote state
// configurations.
func (cfgFile *ConfigFile) Config() (*Config, error) {
	remoteStateConfig, err := ctyhelper.ParseCtyValueToMap(cfgFile.BackendConfig)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}

	cfg.BackendName = cfgFile.BackendName
	if cfgFile.Generate != nil {
		cfg.Generate = &ConfigGenerate{
			Path:     cfgFile.Generate.Path,
			IfExists: cfgFile.Generate.IfExists,
		}
	}

	cfg.BackendConfig = remoteStateConfig

	if cfgFile.Encryption != nil && !cfgFile.Encryption.IsNull() {
		remoteStateEncryption, err := ctyhelper.ParseCtyValueToMap(*cfgFile.Encryption)
		if err != nil {
			return nil, err
		}

		cfg.Encryption = remoteStateEncryption
	} else {
		cfg.Encryption = nil
	}

	if cfgFile.DisableInit != nil {
		cfg.DisableInit = *cfgFile.DisableInit
	}

	if cfgFile.DisableDependencyOptimization != nil {
		cfg.DisableDependencyOptimization = *cfgFile.DisableDependencyOptimization
	}

	return cfg, cfg.Validate()
}
