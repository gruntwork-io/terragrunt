// Package venv defines the root virtualized environment threaded from the
// Terragrunt binary entrypoint down through the CLI and its commands.
//
// A [Venv] bundles the side-effect handles every layer below the CLI needs
// to do its work: [vfs.FS] for filesystem reads and writes, [vexec.Exec]
// for spawning subprocesses, the shell environment variables read at
// startup, and the stdout/stderr writers. Production code constructs the
// real bundle once at the top via [OSVenv]; tests construct an in-memory
// bundle and drive the full CLI through it.
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
	"github.com/gruntwork-io/terragrunt/internal/writer"
)

// Venv is the root virtualized environment. It carries the filesystem,
// process-execution, environment-variable, and writer handles that every
// Terragrunt operation needs.
type Venv struct {
	// FS backs every filesystem read and write.
	FS vfs.FS
	// Exec spawns every subprocess: tofu, terraform, git, hooks,
	// external auth providers, tflint.
	Exec vexec.Exec
	// Env holds the shell environment variables read at startup and is
	// mutated as Terragrunt resolves provider-cache, hook, and inputs
	// env contributions. The map is shared by reference across the run.
	Env map[string]string
	// Writers groups the stdout and stderr handles plus the log-formatting
	// flags that travel together through ParsingContext, shell options,
	// backend options, and the engine.
	Writers writer.Writers
}

// OSVenv builds the production [Venv]: the real OS filesystem, the real
// OS process executor, a snapshot of the OS environment, and stdout/stderr
// wired to the real OS streams.
func OSVenv() Venv {
	return Venv{
		FS:      vfs.NewOSFS(),
		Exec:    vexec.NewOSExec(),
		Env:     parseEnviron(os.Environ()),
		Writers: writer.Writers{Writer: os.Stdout, ErrWriter: os.Stderr},
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
