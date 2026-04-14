package ts

import (
	"runtime"
	"strings"
	"testing"
)

func OnlyOn(tb testing.TB, platforms ...string) {
	tb.Helper()

	thisPlatform := runtime.GOOS + "_" + runtime.GOARCH

	for _, platform := range platforms {
		if strings.HasSuffix(platform, "_") {
			platform += runtime.GOARCH
		}

		if thisPlatform == platform {
			return
		}
	}

	tb.Skipf("skipping test on %s", thisPlatform)
}
