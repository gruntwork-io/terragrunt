package cliconfig

// ProviderInstallation is the structure of the "provider_installation" nested block within the CLI configuration.
type ProviderInstallation struct {
	Methods []ProviderInstallationMethod `hcl:",block"`
}

// ProviderInstallationMethod is an interface type representing the different installation path types and represents an installation method block inside a provider_installation block. The concrete implementations of this interface are:
//
//	ProviderInstallationDirect:           install from the provider's origin registry
//	ProviderInstallationFilesystemMirror: install from a local filesystem mirror
type ProviderInstallationMethod interface {
	AppendExclude(addrs []string)
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

func (method *ProviderInstallationDirect) AppendExclude(addrs []string) {
	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}
	*method.Exclude = append(*method.Exclude, addrs...)
}

type ProviderInstallationFilesystemMirror struct {
	Name    string    `hcl:",label"`
	Path    string    `hcl:"path,attr"`
	Include *[]string `hcl:"include,optional"`
	Exclude *[]string `hcl:"exclude,optional"`
}

func NewProviderInstallationFilesystemMirror(path string, include, exclude []string) *ProviderInstallationFilesystemMirror {
	res := &ProviderInstallationFilesystemMirror{
		Name: "filesystem_mirror",
		Path: path,
	}

	if len(include) > 0 {
		res.Include = &include
	}

	if len(exclude) > 0 {
		res.Exclude = &exclude
	}

	return res
}

func (method *ProviderInstallationFilesystemMirror) AppendExclude(addrs []string) {
	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}
	*method.Exclude = append(*method.Exclude, addrs...)
}

type ProviderInstallationNetworkMirror struct {
	Name    string    `hcl:",label"`
	URL     string    `hcl:"url,attr"`
	Include *[]string `hcl:"include,optional"`
	Exclude *[]string `hcl:"exclude,optional"`
}

func NewProviderInstallationNetworkMirror(url string, include, exclude []string) *ProviderInstallationNetworkMirror {
	res := &ProviderInstallationNetworkMirror{
		Name: "network_mirror",
		URL:  url,
	}

	if len(include) > 0 {
		res.Include = &include
	}

	if len(exclude) > 0 {
		res.Exclude = &exclude
	}

	return res
}

func (method *ProviderInstallationNetworkMirror) AppendExclude(addrs []string) {
	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}
	*method.Exclude = append(*method.Exclude, addrs...)
}
