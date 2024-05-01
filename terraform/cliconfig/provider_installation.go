package cliconfig

type ProviderInstallationFilesystemMirror struct {
	Location string    `hcl:"path,attr"`
	Include  *[]string `hcl:"include,attr"`
	Exclude  *[]string `hcl:"exclude,attr"`
}

func NewProviderInstallationFilesystemMirror(location string, include, exclude []string) *ProviderInstallationFilesystemMirror {
	res := &ProviderInstallationFilesystemMirror{
		Location: location,
	}

	if len(include) > 0 {
		res.Include = &include
	}

	if len(exclude) > 0 {
		res.Exclude = &exclude
	}

	return res
}

type ProviderInstallationDirect struct {
	Include *[]string `hcl:"include,attr"`
	Exclude *[]string `hcl:"exclude,attr"`
}

func NewProviderInstallationDirect(include, exclude []string) *ProviderInstallationDirect {
	res := new(ProviderInstallationDirect)

	if len(include) > 0 {
		res.Include = &include
	}

	if len(exclude) > 0 {
		res.Exclude = &exclude
	}

	return res
}

// ProviderInstallation is the structure of the "provider_installation" nested block within the CLI configuration.
type ProviderInstallation struct {
	FilesystemMirror *ProviderInstallationFilesystemMirror `hcl:"filesystem_mirror,block"`
	Direct           *ProviderInstallationDirect           `hcl:"direct,block"`
}
