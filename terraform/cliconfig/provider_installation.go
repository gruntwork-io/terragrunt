package cliconfig

// ProviderInstallation is the structure of the "provider_installation" nested block within the CLI configuration.
type ProviderInstallation struct {
	Methods []ProviderInstallationMethod `hcl:",block"`
}

// ProviderInstallationMethod is an interface type representing the different installation location types and represents an installation method block inside a provider_installation block. The concrete implementations of this interface are:
//
//	ProviderInstallationDirect:           install from the provider's origin registry
//	ProviderInstallationFilesystemMirror: install from a local filesystem mirror
type ProviderInstallationMethod interface {
	providerInstallationMethod()
}

type ProviderInstallationDirect struct {
	Name    string    `hcl:",label"`
	Include *[]string `hcl:"include,optional"`
	Exclude *[]string `hcl:"exclude,optional"`
}

func NewProviderInstallationDirect(include, exclude []string) *ProviderInstallationDirect {
	res := &ProviderInstallationDirect{
		Name: "direct",
	}

	if len(include) > 0 {
		res.Include = &include
	}

	if len(exclude) > 0 {
		res.Exclude = &exclude
	}

	return res
}

func (ProviderInstallationDirect) providerInstallationMethod() {}

type ProviderInstallationFilesystemMirror struct {
	Name     string    `hcl:",label"`
	Location string    `hcl:"path,attr"`
	Include  *[]string `hcl:"include,optional"`
	Exclude  *[]string `hcl:"exclude,optional"`
}

func NewProviderInstallationFilesystemMirror(location string, include, exclude []string) *ProviderInstallationFilesystemMirror {
	res := &ProviderInstallationFilesystemMirror{
		Name:     "filesystem_mirror",
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

func (ProviderInstallationFilesystemMirror) providerInstallationMethod() {}
