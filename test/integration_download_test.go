package test_test

// The download tests all need a real OpenTofu/Terraform binary and live in
// integration_download_tf_test.go. These fixture paths stay untagged because
// tests outside that file reference them.
const (
	testFixtureLocalDownloadPath                      = "fixtures/download/local"
	testFixtureCustomLockFile                         = "fixtures/download/custom-lock-file"
	testFixtureRemoteDownloadPath                     = "fixtures/download/remote"
	testFixtureInvalidRemoteDownloadPath              = "fixtures/download/remote-invalid"
	testFixtureInvalidRemoteDownloadPathWithRetries   = "fixtures/download/remote-invalid-with-retries"
	testFixtureOverrideDownloadPath                   = "fixtures/download/override"
	testFixtureLocalRelativeDownloadPath              = "fixtures/download/local-relative"
	testFixtureRemoteRelativeDownloadPath             = "fixtures/download/remote-relative"
	testFixtureRemoteRelativeDownloadPathWithSlash    = "fixtures/download/remote-relative-with-slash"
	testFixtureLocalWithExcludeDir                    = "fixtures/download/local-with-exclude-dir"
	testFixtureLocalWithIncludeDir                    = "fixtures/download/local-with-include-dir"
	testFixtureRemoteModuleInRoot                     = "fixtures/download/remote-module-in-root"
	testFixtureLocalMissingBackend                    = "fixtures/download/local-with-missing-backend"
	testFixtureLocalWithHiddenFolder                  = "fixtures/download/local-with-hidden-folder"
	testFixtureLocalWithAllowedHidden                 = "fixtures/download/local-with-allowed-hidden"
	testFixtureLocalPreventDestroy                    = "fixtures/download/local-with-prevent-destroy"
	testFixtureLocalPreventDestroyDependencies        = "fixtures/download/local-with-prevent-destroy-dependencies"
	testFixtureLocalIncludePreventDestroyDependencies = "fixtures/download/local-include-with-prevent-destroy-dependencies"
	testFixtureNotExistingSource                      = "fixtures/download/invalid-path"
	testFixtureDisableCopyLockFilePath                = "fixtures/download/local-disable-copy-terraform-lock-file"
	testFixtureIncludeDisableCopyLockFilePath         = "fixtures/download/local-include-disable-copy-lock-file/module-b"
)
