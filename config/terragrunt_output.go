package config

type TerragruntOutput struct {
	Name       string `hcl:",label"`
	ConfigPath string `hcl:"config_path,attr"`
}
