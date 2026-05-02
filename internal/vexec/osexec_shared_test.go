//go:build exec

// Shared helpers for osexec_posix_test.go and osexec_windows_test.go. Both
// callers are `exec`-gated, so this file must be too; without the tag it
// would compile with no callers.

package vexec_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
)

type parityCase struct {
	desc        string
	name        string
	argv        []string
	wantSuccess bool
}

func runParityCases(t *testing.T, cases []parityCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			// Run real command, capture stdout/stderr/exit separately so the
			// handler can replay them faithfully.
			var osStdout, osStderr bytes.Buffer

			osCmd := vexec.NewOSExec().Command(t.Context(), tc.name, tc.argv...)
			osCmd.SetStdout(&osStdout)
			osCmd.SetStderr(&osStderr)
			osErr := osCmd.Run()

			capturedOut := osStdout.Bytes()
			capturedErr := osStderr.Bytes()
			capturedExit := vexec.ExitCode(osErr)

			// Replay through memCmd.
			memExec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
				return vexec.Result{
					Stdout:   capturedOut,
					Stderr:   capturedErr,
					ExitCode: capturedExit,
				}
			})

			var memStdout, memStderr bytes.Buffer

			memCmd := memExec.Command(t.Context(), tc.name, tc.argv...)
			memCmd.SetStdout(&memStdout)
			memCmd.SetStderr(&memStderr)
			memErr := memCmd.Run()

			// Success/failure shape must match.
			assert.Equal(t, tc.wantSuccess, osErr == nil, "os success mismatch")
			assert.Equal(t, tc.wantSuccess, memErr == nil, "mem success mismatch")

			// Exit codes must be extractable identically.
			assert.Equal(t, vexec.ExitCode(osErr), vexec.ExitCode(memErr), "exit code mismatch")

			// Stream wiring must match byte-for-byte (compare as strings to
			// avoid nil-vs-empty []byte distinctions from bytes.Buffer).
			assert.Equal(t, osStdout.String(), memStdout.String(), "stdout mismatch")
			assert.Equal(t, osStderr.String(), memStderr.String(), "stderr mismatch")

			// CombinedOutput parity: replay through a fresh pair and compare.
			osCombined, osCombErr := vexec.NewOSExec().
				Command(t.Context(), tc.name, tc.argv...).
				CombinedOutput()

			memCombined, memCombErr := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
				return vexec.Result{
					Stdout:   capturedOut,
					Stderr:   capturedErr,
					ExitCode: capturedExit,
				}
			}).Command(t.Context(), tc.name, tc.argv...).CombinedOutput()

			assert.Equal(t, tc.wantSuccess, osCombErr == nil)
			assert.Equal(t, tc.wantSuccess, memCombErr == nil)
			assert.Equal(t, vexec.ExitCode(osCombErr), vexec.ExitCode(memCombErr))
			assert.Equal(t, string(osCombined), string(memCombined), "combined output mismatch")
		})
	}
}
