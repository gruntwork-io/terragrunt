package flags

const (
	// TgPrefix is an environment variable prefix.
	TgPrefix = "TG"

	// TerragruntPrefix is an environment variable deprecated prefix.
	TerragruntPrefix = "TERRAGRUNT"
)

func ConfigKey(name string) string {
	return Name{}.ConfigKey(name)
}

func EnvVarsWithTgPrefix(names ...string) []string {
	return Name{TgPrefix}.EnvVars(names...)
}

func EnvVarsWithTerragruntPrefix(names ...string) []string {
	return Name{TerragruntPrefix}.EnvVars(names...)
}

func FlagNamesWithTerragruntPrefix(names ...string) []string {
	return Name{TerragruntPrefix}.FlagNames(names...)
}

func FlagNameWithTerragruntPrefix(name string) string {
	return Name{TerragruntPrefix}.FlagName(name)
}
