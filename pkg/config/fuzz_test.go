package config_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// FuzzHCLStringHelpers: startswith / endswith / strcontains must never panic on any args shape.
func FuzzHCLStringHelpers(f *testing.F) {
	seeds := []string{
		"",
		"foo",
		"foo\x00bar",
		"foo\x00bar\x00baz",
		"\x00",
		"\x00\x00",
		"a\x00b\x00c\x00d\x00e",
		"hello world\x00world",
		"hello world\x00hello",
		"hello world\x00wor",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		var args []string
		if raw != "" {
			args = strings.Split(raw, "\x00")
		}

		ctx, pctx := newTestParsingContext(t, "")

		_, _ = config.StartsWith(ctx, pctx, args)
		_, _ = config.EndsWith(ctx, pctx, args)
		_, _ = config.StrContains(ctx, pctx, args)
	})
}

// FuzzHCLRunCommandOptions: run_cmd must never panic on any mix of option flags and commands.
func FuzzHCLRunCommandOptions(f *testing.F) {
	if runtime.GOOS == "windows" {
		f.Skip("run_cmd happy-path requires bash; skip on Windows")
	}

	seeds := []string{
		"--terragrunt-quiet",
		"--terragrunt-no-cache",
		"--terragrunt-global-cache",
		"--terragrunt-quiet\x00--terragrunt-no-cache",
		"--terragrunt-quiet\x00--terragrunt-quiet",
		"--terragrunt-quiet\x00/bin/echo\x00hi",
		"/bin/echo\x00hi",
		"--terragrunt-no-cache\x00--terragrunt-global-cache",
		"--unknown-flag",
		"",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		var args []string
		if raw != "" {
			args = strings.Split(raw, "\x00")
		}

		l := logger.CreateLogger()
		ctx, pctx := newTestParsingContext(t, "")

		_, _ = config.RunCommand(ctx, pctx, l, args)
	})
}
