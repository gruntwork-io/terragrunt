package ts

import (
	"runtime"
	"strings"
	"testing"
)

func OnlyOn(t testing.TB, platforms ...string) {
	t.Helper()

	thisPlatform := runtime.GOOS + "_" + runtime.GOARCH

	for _, platform := range platforms {
		if strings.HasSuffix(platform, "_") {
			platform = platform + runtime.GOARCH
		}
		if thisPlatform == platform {
			return
		}
	}
	t.Skipf("skipping test on %s", thisPlatform)
}
