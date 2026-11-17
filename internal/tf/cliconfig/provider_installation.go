package cliconfig

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/gruntwork-io/terragrunt/internal/util"
)

// ProviderInstallation is the structure of the "provider_installation" nested block within the CLI configuration.
type ProviderInstallation struct {
	Methods ProviderInstallationMethods `hcl:",block"`
}

// ProviderInstallationMethods is the list of method blocks in a `provider_installation` block.
type ProviderInstallationMethods []ProviderInstallationMethod

// Merge folds each of withMethods into the existing method of the same kind, and for mirrors the
// same path or URL, and appends the rest. It then moves `direct` methods to the end.
//
// Merge modifies the matching methods of the receiver in place.
func (methods ProviderInstallationMethods) Merge(
	withMethods ...ProviderInstallationMethod,
) ProviderInstallationMethods {
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

// Clone returns a deep copy of methods.
func (methods ProviderInstallationMethods) Clone() ProviderInstallationMethods {
	var cloned = make(ProviderInstallationMethods, len(methods))

	for i, method := range methods {
		cloned[i] = method.Clone()
	}

	return cloned
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
	Clone() ProviderInstallationMethod
}

// ProviderInstallationDirect is the `direct` method block, which installs providers from their origin registry.
type ProviderInstallationDirect struct {
	Include *[]string `hcl:"include,optional" json:"Include"`
	Exclude *[]string `hcl:"exclude,optional" json:"Exclude"`
	Name    string    `hcl:",label"           json:"Name"`
}

// NewProviderInstallationDirect returns a `direct` method limited by include and exclude. An empty
// list leaves its attribute unset.
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

// Clone returns a deep copy of method.
func (method *ProviderInstallationDirect) Clone() ProviderInstallationMethod {
	cloned := &ProviderInstallationDirect{
		Name: method.Name,
	}

	if method.Include != nil {
		include := *method.Include
		cloned.Include = &include
	}

	if method.Exclude != nil {
		exclude := *method.Exclude
		cloned.Exclude = &exclude
	}

	return cloned
}

// Merge adds the include and exclude lists of with to method when with is also a `direct` method,
// and reports whether it did.
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

// AppendInclude adds addrs to the include list, which it keeps sorted and free of duplicates.
func (method *ProviderInstallationDirect) AppendInclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Include == nil {
		method.Include = &[]string{}
	}

	*method.Include = util.RemoveDuplicates(append(*method.Include, addrs...))
}

// AppendExclude adds addrs to the exclude list, which it keeps sorted and free of duplicates.
func (method *ProviderInstallationDirect) AppendExclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}

	*method.Exclude = util.RemoveDuplicates(append(*method.Exclude, addrs...))
}

// RemoveExclude removes addrs from the exclude list, unsetting the attribute when the list empties.
func (method *ProviderInstallationDirect) RemoveExclude(addrs []string) {
	if len(addrs) == 0 || method.Exclude == nil {
		return
	}

	*method.Exclude = slices.DeleteFunc(
		*method.Exclude,
		func(item string) bool { return slices.Contains(addrs, item) },
	)

	if len(*method.Exclude) == 0 {
		method.Exclude = nil
	}
}

// RemoveInclude removes addrs from the include list, unsetting the attribute when the list empties.
func (method *ProviderInstallationDirect) RemoveInclude(addrs []string) {
	if len(addrs) == 0 || method.Include == nil {
		return
	}

	*method.Include = slices.DeleteFunc(
		*method.Include,
		func(item string) bool { return slices.Contains(addrs, item) },
	)

	if len(*method.Include) == 0 {
		method.Include = nil
	}
}

func (method *ProviderInstallationDirect) String() string {
	b, err := json.Marshal(method)
	if err != nil {
		panic(fmt.Sprintf("json.Marshal cannot fail on %T, whose fields are strings and string slices: %v", method, err))
	}

	return string(b)
}

// ProviderInstallationFilesystemMirror is the `filesystem_mirror` method block, which installs providers from a local directory.
type ProviderInstallationFilesystemMirror struct {
	Include *[]string `hcl:"include,optional" json:"Include"`
	Exclude *[]string `hcl:"exclude,optional" json:"Exclude"`
	Name    string    `hcl:",label"           json:"Name"`
	Path    string    `hcl:"path,attr"        json:"Path"`
}

// NewProviderInstallationFilesystemMirror returns a `filesystem_mirror` method that reads from path,
// limited by include and exclude. An empty list leaves its attribute unset.
func NewProviderInstallationFilesystemMirror(
	path string,
	include, exclude []string,
) *ProviderInstallationFilesystemMirror {
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

// Clone returns a deep copy of method.
func (method *ProviderInstallationFilesystemMirror) Clone() ProviderInstallationMethod {
	cloned := &ProviderInstallationFilesystemMirror{
		Name: method.Name,
		Path: method.Path,
	}

	if method.Include != nil {
		include := *method.Include
		cloned.Include = &include
	}

	if method.Exclude != nil {
		exclude := *method.Exclude
		cloned.Exclude = &exclude
	}

	return cloned
}

// Merge adds the include and exclude lists of with to method when with is a `filesystem_mirror`
// method with the same path, and reports whether it did.
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

// AppendInclude adds addrs to the include list, which it keeps sorted and free of duplicates.
func (method *ProviderInstallationFilesystemMirror) AppendInclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Include == nil {
		method.Include = &[]string{}
	}

	*method.Include = util.RemoveDuplicates(append(*method.Include, addrs...))
}

// AppendExclude adds addrs to the exclude list, which it keeps sorted and free of duplicates.
func (method *ProviderInstallationFilesystemMirror) AppendExclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}

	*method.Exclude = util.RemoveDuplicates(append(*method.Exclude, addrs...))
}

// RemoveExclude removes addrs from the exclude list, unsetting the attribute when the list empties.
func (method *ProviderInstallationFilesystemMirror) RemoveExclude(addrs []string) {
	if len(addrs) == 0 || method.Exclude == nil {
		return
	}

	*method.Exclude = slices.DeleteFunc(
		*method.Exclude,
		func(item string) bool { return slices.Contains(addrs, item) },
	)

	if len(*method.Exclude) == 0 {
		method.Exclude = nil
	}
}

// RemoveInclude removes addrs from the include list, unsetting the attribute when the list empties.
func (method *ProviderInstallationFilesystemMirror) RemoveInclude(addrs []string) {
	if len(addrs) == 0 || method.Include == nil {
		return
	}

	*method.Include = slices.DeleteFunc(
		*method.Include,
		func(item string) bool { return slices.Contains(addrs, item) },
	)

	if len(*method.Include) == 0 {
		method.Include = nil
	}
}

func (method *ProviderInstallationFilesystemMirror) String() string {
	b, err := json.Marshal(method)
	if err != nil {
		panic(fmt.Sprintf("json.Marshal cannot fail on %T, whose fields are strings and string slices: %v", method, err))
	}

	return string(b)
}

// ProviderInstallationNetworkMirror is the `network_mirror` method block, which installs providers from a mirror server.
type ProviderInstallationNetworkMirror struct {
	Include *[]string `hcl:"include,optional" json:"Include"`
	Exclude *[]string `hcl:"exclude,optional" json:"Exclude"`
	Name    string    `hcl:",label"           json:"Name"`
	URL     string    `hcl:"url,attr"         json:"URL"`
}

// NewProviderInstallationNetworkMirror returns a `network_mirror` method that reads from url,
// limited by include and exclude. An empty list leaves its attribute unset.
func NewProviderInstallationNetworkMirror(
	url string,
	include, exclude []string,
) *ProviderInstallationNetworkMirror {
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

// Clone returns a deep copy of method.
func (method *ProviderInstallationNetworkMirror) Clone() ProviderInstallationMethod {
	cloned := &ProviderInstallationNetworkMirror{
		Name: method.Name,
		URL:  method.URL,
	}

	if method.Include != nil {
		include := *method.Include
		cloned.Include = &include
	}

	if method.Exclude != nil {
		exclude := *method.Exclude
		cloned.Exclude = &exclude
	}

	return cloned
}

// Merge adds the include and exclude lists of with to method when with is a `network_mirror`
// method with the same URL, and reports whether it did.
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

// AppendInclude adds addrs to the include list, which it keeps sorted and free of duplicates.
func (method *ProviderInstallationNetworkMirror) AppendInclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Include == nil {
		method.Include = &[]string{}
	}

	*method.Include = util.RemoveDuplicates(append(*method.Include, addrs...))
}

// AppendExclude adds addrs to the exclude list, dropping duplicates and keeping the last
// occurrence of each.
func (method *ProviderInstallationNetworkMirror) AppendExclude(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	if method.Exclude == nil {
		method.Exclude = &[]string{}
	}

	*method.Exclude = util.RemoveDuplicatesKeepLast(append(*method.Exclude, addrs...))
}

// RemoveExclude removes addrs from the exclude list, unsetting the attribute when the list empties.
func (method *ProviderInstallationNetworkMirror) RemoveExclude(addrs []string) {
	if len(addrs) == 0 || method.Exclude == nil {
		return
	}

	*method.Exclude = slices.DeleteFunc(
		*method.Exclude,
		func(item string) bool { return slices.Contains(addrs, item) },
	)

	if len(*method.Exclude) == 0 {
		method.Exclude = nil
	}
}

// RemoveInclude removes addrs from the include list, unsetting the attribute when the list empties.
func (method *ProviderInstallationNetworkMirror) RemoveInclude(addrs []string) {
	if len(addrs) == 0 || method.Include == nil {
		return
	}

	*method.Include = slices.DeleteFunc(
		*method.Include,
		func(item string) bool { return slices.Contains(addrs, item) },
	)

	if len(*method.Include) == 0 {
		method.Include = nil
	}
}

func (method *ProviderInstallationNetworkMirror) String() string {
	b, err := json.Marshal(method)
	if err != nil {
		panic(fmt.Sprintf("json.Marshal cannot fail on %T, whose fields are strings and string slices: %v", method, err))
	}

	return string(b)
}
