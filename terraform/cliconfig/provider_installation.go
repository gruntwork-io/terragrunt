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
	mergedMethods := methods

	for _, withMethod := range withMethods {
		var isMerged bool

		for _, method := range methods {
			if method.Merge(withMethod) {
				isMerged = true
				break
			}
		}

		if !isMerged {
			mergedMethods = append(mergedMethods, withMethod)
		}
	}

	// place the `direct` method at the very end.
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
	RemoveInclude(addrs []string)
	RemoveExclude(addrs []string)
	Merge(with ProviderInstallationMethod) bool
}

type ProviderInstallationDirect struct {
	Name    string    `hcl:",label" json:"Name"`
	Include *[]string `hcl:"include,optional" json:"Include"`
	Exclude *[]string `hcl:"exclude,optional" json:"Exclude"`
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

func (method *ProviderInstallationDirect) Merge(with ProviderInstallationMethod) bool {
	if with, ok := with.(*ProviderInstallationDirect); ok {
		if with.Exclude != nil {
			method.AppendExclude(*with.Exclude)
		}

		if with.Include != nil {
			method.AppendInclude(*with.Include)
		}

		return true
	}

	return false
}

func (method *ProviderInstallationDirect) AppendInclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Include == nil {
		method.Include = &[]string{}
	}

	*method.Include = util.RemoveDuplicatesFromList(append(*method.Include, addrs...))
}

func (method *ProviderInstallationDirect) AppendExclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}

	*method.Exclude = util.RemoveDuplicatesFromList(append(*method.Exclude, addrs...))
}

func (method *ProviderInstallationDirect) RemoveExclude(addrs []string) {
	if len(addrs) == 0 || method.Exclude == nil {
		return
	}

	*method.Exclude = util.RemoveSublistFromList(*method.Exclude, addrs)

	if len(*method.Exclude) == 0 {
		method.Exclude = nil
	}
}

func (method *ProviderInstallationDirect) RemoveInclude(addrs []string) {
	if len(addrs) == 0 || method.Include == nil {
		return
	}

	*method.Include = util.RemoveSublistFromList(*method.Include, addrs)

	if len(*method.Include) == 0 {
		method.Include = nil
	}
}

func (method *ProviderInstallationDirect) String() string {
	// Odd that this err isn't checked. There should be an explanation why.
	b, _ := json.Marshal(method) //nolint:errchkjson
	return string(b)
}

type ProviderInstallationFilesystemMirror struct {
	Name    string    `hcl:",label" json:"Name"`
	Path    string    `hcl:"path,attr" json:"Path"`
	Include *[]string `hcl:"include,optional" json:"Include"`
	Exclude *[]string `hcl:"exclude,optional" json:"Exclude"`
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

func (method *ProviderInstallationFilesystemMirror) Merge(with ProviderInstallationMethod) bool {
	if with, ok := with.(*ProviderInstallationFilesystemMirror); ok && method.Path == with.Path {
		if with.Exclude != nil {
			method.AppendExclude(*with.Exclude)
		}

		if with.Include != nil {
			method.AppendInclude(*with.Include)
		}

		return true
	}

	return false
}

func (method *ProviderInstallationFilesystemMirror) AppendInclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Include == nil {
		method.Include = &[]string{}
	}

	*method.Include = util.RemoveDuplicatesFromList(append(*method.Include, addrs...))
}

func (method *ProviderInstallationFilesystemMirror) AppendExclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}

	*method.Exclude = util.RemoveDuplicatesFromList(append(*method.Exclude, addrs...))
}

func (method *ProviderInstallationFilesystemMirror) RemoveExclude(addrs []string) {
	if len(addrs) == 0 || method.Exclude == nil {
		return
	}

	*method.Exclude = util.RemoveSublistFromList(*method.Exclude, addrs)

	if len(*method.Exclude) == 0 {
		method.Exclude = nil
	}
}

func (method *ProviderInstallationFilesystemMirror) RemoveInclude(addrs []string) {
	if len(addrs) == 0 || method.Include == nil {
		return
	}

	*method.Include = util.RemoveSublistFromList(*method.Include, addrs)

	if len(*method.Include) == 0 {
		method.Include = nil
	}
}

func (method *ProviderInstallationFilesystemMirror) String() string {
	// Odd that this err isn't checked. There should be an explanation why.
	b, _ := json.Marshal(method) //nolint:errchkjson
	return string(b)
}

type ProviderInstallationNetworkMirror struct {
	Name    string    `hcl:",label" json:"Name"`
	URL     string    `hcl:"url,attr" json:"URL"`
	Include *[]string `hcl:"include,optional" json:"Include"`
	Exclude *[]string `hcl:"exclude,optional" json:"Exclude"`
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

func (method *ProviderInstallationNetworkMirror) Merge(with ProviderInstallationMethod) bool {
	if with, ok := with.(*ProviderInstallationNetworkMirror); ok && method.URL == with.URL {
		if with.Exclude != nil {
			method.AppendExclude(*with.Exclude)
		}

		if with.Include != nil {
			method.AppendInclude(*with.Include)
		}

		return true
	}

	return false
}

func (method *ProviderInstallationNetworkMirror) AppendInclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Include == nil {
		method.Include = &[]string{}
	}

	*method.Include = util.RemoveDuplicatesFromList(append(*method.Include, addrs...))
}

func (method *ProviderInstallationNetworkMirror) AppendExclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}

	*method.Exclude = util.RemoveDuplicatesFromList(append(*method.Exclude, addrs...))
}

func (method *ProviderInstallationNetworkMirror) RemoveExclude(addrs []string) {
	if len(addrs) == 0 || method.Exclude == nil {
		return
	}

	*method.Exclude = util.RemoveSublistFromList(*method.Exclude, addrs)

	if len(*method.Exclude) == 0 {
		method.Exclude = nil
	}
}

func (method *ProviderInstallationNetworkMirror) RemoveInclude(addrs []string) {
	if len(addrs) == 0 || method.Include == nil {
		return
	}

	*method.Include = util.RemoveSublistFromList(*method.Include, addrs)

	if len(*method.Include) == 0 {
		method.Include = nil
	}
}

func (method *ProviderInstallationNetworkMirror) String() string {
	// Odd that this err isn't checked. There should be an explanation why.
	b, _ := json.Marshal(method) //nolint:errchkjson
	return string(b)
}
