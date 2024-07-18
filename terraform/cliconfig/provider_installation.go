package cliconfig

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gruntwork-io/terragrunt/util"
)

// ProviderInstallation is the structure of the "provider_installation" nested block within the CLI configuration.
type ProviderInstallation struct {
	Methods ProviderInstallationMethods `hcl:",block"`
}

type ProviderInstallationMethods []ProviderInstallationMethod

func (methods ProviderInstallationMethods) Merge(withMethods ...ProviderInstallationMethod) ProviderInstallationMethods {
	for _, method := range methods {
		remainedWithMethods := withMethods
		withMethods = ProviderInstallationMethods{}

		for _, withMethod := range remainedWithMethods {
			var isMerged bool

			if method, ok := method.(*ProviderInstallationFilesystemMirror); ok {
				if withMethod, ok := withMethod.(*ProviderInstallationFilesystemMirror); ok && method.Path == withMethod.Path {
					if withMethod.Exclude != nil {
						method.AppendExclude(*withMethod.Exclude)
					}
					if withMethod.Include != nil {
						method.AppendInclude(*withMethod.Include)
					}
					isMerged = true
				}
			}

			if method, ok := method.(*ProviderInstallationNetworkMirror); ok {
				if withMethod, ok := withMethod.(*ProviderInstallationNetworkMirror); ok && method.URL == withMethod.URL {
					if withMethod.Exclude != nil {
						method.AppendExclude(*withMethod.Exclude)
					}
					if withMethod.Include != nil {
						method.AppendInclude(*withMethod.Include)
					}
					isMerged = true
				}
			}

			if method, ok := method.(*ProviderInstallationDirect); ok {
				if withMethod, ok := withMethod.(*ProviderInstallationDirect); ok {
					if withMethod.Exclude != nil {
						method.AppendExclude(*withMethod.Exclude)
					}
					if withMethod.Include != nil {
						method.AppendInclude(*withMethod.Include)
					}
					isMerged = true
				}
			}

			if !isMerged {
				withMethods = append(withMethods, withMethod)
			}
		}
	}

	mergedMethods := append(methods, withMethods...)

	sort.Slice(mergedMethods, func(i, j int) bool {
		if _, ok := mergedMethods[j].(*ProviderInstallationDirect); ok {
			return true
		}
		return false
	})

	return mergedMethods
}

// ProviderInstallationMethod is an interface type representing the different installation path types and represents an installation method block inside a provider_installation block. The concrete implementations of this interface are:
//
//	ProviderInstallationDirect:           install from the provider's origin registry
//	ProviderInstallationFilesystemMirror: install from a local filesystem mirror
type ProviderInstallationMethod interface {
	fmt.Stringer
	AppendInclude(addrs []string)
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

func (method *ProviderInstallationDirect) AppendInclude(addrs []string) {
	if method.Include == nil {
		method.Include = &[]string{}
	}
	*method.Include = util.RemoveDuplicatesFromList(append(*method.Include, addrs...))
}

func (method *ProviderInstallationDirect) AppendExclude(addrs []string) {
	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}
	*method.Exclude = util.RemoveDuplicatesFromList(append(*method.Exclude, addrs...))
}

func (method *ProviderInstallationDirect) String() string {
	b, _ := json.Marshal(method) //nolint:errcheck
	return string(b)
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

func (method *ProviderInstallationFilesystemMirror) AppendInclude(addrs []string) {
	if method.Include == nil {
		method.Include = &[]string{}
	}
	*method.Include = util.RemoveDuplicatesFromList(append(*method.Include, addrs...))
}

func (method *ProviderInstallationFilesystemMirror) AppendExclude(addrs []string) {
	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}
	*method.Exclude = util.RemoveDuplicatesFromList(append(*method.Exclude, addrs...))
}

func (method *ProviderInstallationFilesystemMirror) String() string {
	b, _ := json.Marshal(method) //nolint:errcheck
	return string(b)
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

func (method *ProviderInstallationNetworkMirror) AppendInclude(addrs []string) {
	if method.Include == nil {
		method.Include = &[]string{}
	}
	*method.Include = util.RemoveDuplicatesFromList(append(*method.Include, addrs...))
}

func (method *ProviderInstallationNetworkMirror) AppendExclude(addrs []string) {
	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}
	*method.Exclude = util.RemoveDuplicatesFromList(append(*method.Exclude, addrs...))
}

func (method *ProviderInstallationNetworkMirror) String() string {
	b, _ := json.Marshal(method) //nolint:errcheck
	return string(b)
}
