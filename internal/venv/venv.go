// Package venv defines the root virtualized environment threaded from the
// Terragrunt binary entrypoint down through the CLI and its commands.
//
// A [Venv] bundles the side-effect handles every layer below the CLI needs
// to do its work: [vfs.FS] for filesystem reads and writes, [vexec.Exec]
// for spawning subprocesses, [vhttp.Client] for outbound HTTP (including
// the transport handed to AWS and GCP SDK builders via
// [github.com/gruntwork-io/terragrunt/internal/awshelper.AWSConfigBuilder.WithHTTPClient]
// and [github.com/gruntwork-io/terragrunt/internal/gcphelper.GCPConfigBuilder.WithHTTPClient]),
// the shell environment variables read at startup, and the stdout/stderr
// writers. Production code constructs the real bundle once at the top via
// [OSVenv]; tests construct an in-memory bundle and drive the full CLI
// through it.
//
// Downstream packages (for example internal/runner/run and internal/tflint)
// keep their own package-local Venv types so each owns its own contract.
// They convert from this root via a FromRoot constructor.
package venv

import (
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// Venv is the root virtualized environment. It carries the filesystem,
// process-execution, HTTP, environment-variable, and writer handles that
// every Terragrunt operation needs.
type Venv struct {
	// FS backs every filesystem read and write.
	FS vfs.FS
	// Exec spawns every subprocess: tofu, terraform, git, hooks,
	// external auth providers, tflint.
	Exec vexec.Exec
	// HTTP performs every outbound HTTP request. AWS and GCP SDK builders
	// receive it via their respective WithHTTPClient methods so test
	// substitution at the [net/http.RoundTripper] boundary covers cloud
	// traffic uniformly with plain HTTP traffic.
	HTTP vhttp.Client
	// Env holds the shell environment variables read at startup and is
	// mutated as Terragrunt resolves provider-cache, hook, and inputs
	// env contributions. The map is shared by reference across the run.
	Env map[string]string
	// Writers groups the stdout and stderr handles that travel together
	// through ParsingContext, shell options, backend options, and the
	// engine. It is held as a pointer so per-call overrides via
	// [writer.Writers.WithWriter] and [writer.Writers.WithErrWriter]
	// produce fresh pointers without mutating the caller's value.
	// Direct field mutation (e.g. `v.Writers.Writer = …`) is unsafe
	// because shallow-copying a [Venv] shares this pointer with the
	// original; always replace via With* helpers instead.
	Writers *writer.Writers
}

// OSVenv builds the production [Venv]: the real OS filesystem, the real
// OS process executor, the real outbound HTTP client, a snapshot of the
// OS environment, and stdout/stderr wired to the real OS streams.
//
// It returns a *[Venv] so the bundle is threaded by pointer through every
// downstream call — small parameter, no copying, and shallow-copying a
// pointed-to [Venv] (via `local := *v`) yields an independent value that
// callers can mutate (Env, Writers via [writer.Writers.WithWriter]) without
// affecting the original.
func OSVenv() *Venv {
	return &Venv{
		FS:      vfs.NewOSFS(),
		Exec:    vexec.NewOSExec(),
		HTTP:    vhttp.NewOSClient(),
		Env:     parseEnviron(os.Environ()),
		Writers: &writer.Writers{Writer: os.Stdout, ErrWriter: os.Stderr},
	}
}

// parseEnviron converts a slice of KEY=VALUE strings (the shape returned
// by os.Environ) into a map. Entries without an "=" are treated as keys
// with empty values, matching the convention used elsewhere in the tree.
func parseEnviron(environ []string) map[string]string {
	out := make(map[string]string, len(environ))

	for _, entry := range environ {
		key, value, _ := strings.Cut(entry, "=")
		out[key] = value
	}

	return out
}
