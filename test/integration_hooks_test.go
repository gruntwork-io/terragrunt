package test_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testFixtureHooksBeforeOnlyPath                                = "fixtures/hooks/before-only"
	testFixtureHooksAllPath                                       = "fixtures/hooks/all"
	testFixtureHooksAfterOnlyPath                                 = "fixtures/hooks/after-only"
	testFixtureHooksBeforeAndAfterPath                            = "fixtures/hooks/before-and-after"
	testFixtureHooksSkipOnErrorPath                               = "fixtures/hooks/skip-on-error"
	testFixtureErrorHooksPath                                     = "fixtures/hooks/error-hooks"
	testFixtureErrorHooksSourceDownloadFail                       = "fixtures/hooks/error-hooks-source-download-fail"
	testFixtureHooksOneArgActionPath                              = "fixtures/hooks/one-arg-action"
	testFixtureHooksEmptyStringCommandPath                        = "fixtures/hooks/bad-arg-action/empty-string-command"
	testFixtureHooksEmptyCommandListPath                          = "fixtures/hooks/bad-arg-action/empty-command-list"
	testFixtureHooksInterpolationsPath                            = "fixtures/hooks/interpolations"
	testFixtureHooksInitOnceNoSourceNoBackend                     = "fixtures/hooks/init-once/no-source-no-backend"
	testFixtureHooksInitOnceWithSourceNoBackend                   = "fixtures/hooks/init-once/with-source-no-backend"
	testFixtureHooksInitOnceWithSourceNoBackendSuppressHookStdout = "fixtures/hooks/init-once/with-source-no-backend-suppress-hook-stdout"
	testFixtureTerragruntHookIfParameter                          = "fixtures/hooks/if-parameter"
	testFixtureHooksPathPreservation                              = "fixtures/hooks/path-preservation"
	testFixtureHooksExitCodeError                                 = "fixtures/hooks/exit-code-error"
	testFixtureHooksContextEnv                                    = "fixtures/hooks/hook-context-env"
	testFixtureHooksNoHooks                                       = "fixtures/hooks/no-hooks"
)

func assertNoHookOutputFiles(t *testing.T, unitPaths ...string) {
	t.Helper()

	for _, unitPath := range unitPaths {
		assert.NoFileExists(t, filepath.Join(unitPath, "before.out"))
		assert.NoFileExists(t, filepath.Join(unitPath, "after.out"))
		assert.NoFileExists(t, filepath.Join(unitPath, "error.out"))
	}
}
