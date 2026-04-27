package config_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// FuzzHCLStringHelpers: wrong arity must return WrongNumberOfParamsError; otherwise the result must agree with the Go stdlib equivalent.
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
		args := strings.Split(raw, "\x00")

		ctx, pctx := newTestParsingContext(t, "")

		swOut, swErr := config.StartsWith(ctx, pctx, args)
		ewOut, ewErr := config.EndsWith(ctx, pctx, args)
		scOut, scErr := config.StrContains(ctx, pctx, args)

		if len(args) != 2 {
			require.Error(t, swErr, "startswith with %d args must error", len(args))
			require.True(t, assertErrorType(t, config.WrongNumberOfParamsError{}, swErr),
				"startswith expected WrongNumberOfParamsError, got %T: %v", swErr, swErr)
			require.Error(t, ewErr, "endswith with %d args must error", len(args))
			require.True(t, assertErrorType(t, config.WrongNumberOfParamsError{}, ewErr),
				"endswith expected WrongNumberOfParamsError, got %T: %v", ewErr, ewErr)
			require.Error(t, scErr, "strcontains with %d args must error", len(args))
			require.True(t, assertErrorType(t, config.WrongNumberOfParamsError{}, scErr),
				"strcontains expected WrongNumberOfParamsError, got %T: %v", scErr, scErr)

			return
		}

		require.NoError(t, swErr, "startswith(%q,%q) must not error", args[0], args[1])
		require.Equal(t, strings.HasPrefix(args[0], args[1]), swOut,
			"startswith(%q,%q) must agree with strings.HasPrefix", args[0], args[1])

		require.NoError(t, ewErr, "endswith(%q,%q) must not error", args[0], args[1])
		require.Equal(t, strings.HasSuffix(args[0], args[1]), ewOut,
			"endswith(%q,%q) must agree with strings.HasSuffix", args[0], args[1])

		require.NoError(t, scErr, "strcontains(%q,%q) must not error", args[0], args[1])
		require.Equal(t, strings.Contains(args[0], args[1]), scOut,
			"strcontains(%q,%q) must agree with strings.Contains", args[0], args[1])
	})
}

// FuzzRunCmdOptionsParsing: exercises run_cmd's option-stripping logic without ever reaching shell-out.
// Args are filtered to known --terragrunt-* flags and NO command is appended, so RunCommand always returns
// EmptyStringNotAllowedError or ConflictingRunCmdCacheOptionsError before shell.RunCommandWithOutput is called.
func FuzzRunCmdOptionsParsing(f *testing.F) {
	seeds := []string{
		"",
		"--terragrunt-quiet",
		"--terragrunt-no-cache",
		"--terragrunt-global-cache",
		"--terragrunt-quiet\x00--terragrunt-no-cache",
		"--terragrunt-quiet\x00--terragrunt-quiet",
		"--terragrunt-no-cache\x00--terragrunt-global-cache",
		"--terragrunt-global-cache\x00--terragrunt-no-cache",
		"--terragrunt-quiet\x00--terragrunt-no-cache\x00--terragrunt-global-cache",
		"--unknown-flag",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		parts := strings.Split(raw, "\x00")

		var hasNoCache, hasGlobalCache bool

		args := make([]string, 0, len(parts))
		for _, part := range parts {
			switch part {
			case "--terragrunt-quiet":
				args = append(args, part)
			case "--terragrunt-no-cache":
				args = append(args, part)
				hasNoCache = true
			case "--terragrunt-global-cache":
				args = append(args, part)
				hasGlobalCache = true
			}
		}

		baseCtx, pctx := newTestParsingContext(t, "")

		ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
		defer cancel()

		l := logger.CreateLogger()

		out, err := config.RunCommand(ctx, pctx, l, args)

		require.Empty(t, out, "options-only call must not produce output")
		require.Error(t, err, "options-only call must error")

		if hasNoCache && hasGlobalCache {
			require.True(t, assertErrorType(t, config.ConflictingRunCmdCacheOptionsError{}, err),
				"expected ConflictingRunCmdCacheOptionsError, got %T: %v", err, err)

			return
		}

		require.True(t, assertErrorType(t, config.EmptyStringNotAllowedError(""), err),
			"expected EmptyStringNotAllowedError, got %T: %v", err, err)
	})
}
