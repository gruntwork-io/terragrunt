// Package getter is Terragrunt's single integration point with the
// hashicorp/go-getter/v2 library. It owns the default protocol registration
// (including the s3/v2 and gcs/v2 sub-modules), hosts the Terragrunt-specific
// custom getters (FileCopyGetter, GitGetter, HgGetter, SmbClientGetter,
// SmbMountGetter, RegistryGetter, CASGetter, CASProtocolGetter), and
// re-exports the small subset of v2 types other packages need.
//
// Every getter that runs a program runs it through the venv's exec rather
// than go-getter's own os/exec calls, so an exec that refuses or restricts
// subprocesses governs source downloads too.
//
// New code outside internal/getter should depend on this package rather than
// hashicorp/go-getter/v2 directly. Two packages have unavoidable direct
// imports of go-getter helpers and are explicitly carved out:
//
//   - internal/util/file.go uses helper/url.Parse for one URL parse. Routing
//     it through internal/getter would create a cycle (internal/getter
//     imports internal/util for util.CopyFolderContents).
//   - internal/cas/stacks.go uses go-getter.SourceDirSubdir. Routing it
//     through internal/getter would create a cycle (internal/getter imports
//     internal/cas for the CAS getters).
//
// Both call only stateless helpers (URL parsing and "//subdir" splitting),
// neither of which interacts with the registered protocol set, so they don't
// affect the goal of single-source protocol registration.
package getter
