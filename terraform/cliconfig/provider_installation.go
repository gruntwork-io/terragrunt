package cliconfig

// ProviderInstallationMethod represents an installation method block inside a provider_installation block.
type ProviderInstallationMethod struct {
	Include []string `hcl:"include"`
	Exclude []string `hcl:"exclude"`
}

type ProviderInstallationFilesystemMirror struct {
	Location string `hcl:"path"`
	*ProviderInstallationMethod
}

func NewProviderInstallationFilesystemMirror(location string, include, exclude []string) *ProviderInstallationFilesystemMirror {
	return &ProviderInstallationFilesystemMirror{
		Location: location,
		ProviderInstallationMethod: &ProviderInstallationMethod{
			Include: include,
			Exclude: exclude,
		},
	}
}

type ProviderInstallationDirect struct {
	*ProviderInstallationMethod
}

func NewProviderInstallationDirect(include, exclude []string) *ProviderInstallationDirect {
	return &ProviderInstallationDirect{
		ProviderInstallationMethod: &ProviderInstallationMethod{
			Include: include,
			Exclude: exclude,
		},
	}
}

// ProviderInstallation is the structure of the "provider_installation" nested block within the CLI configuration.
type ProviderInstallation struct {
	FilesystemMirror *ProviderInstallationFilesystemMirror `hcl:"filesystem_mirror"`
	Direct           *ProviderInstallationDirect           `hcl:"direct"`
}
