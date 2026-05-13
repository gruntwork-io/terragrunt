// Package venv defines the root virtualized environment threaded from the
// Terragrunt binary entrypoint down through the CLI and its commands.
//
// A [Venv] bundles the two side-effect handles every layer below the CLI
// needs to do its work: [vfs.FS] for filesystem reads and writes, and
// [vexec.Exec] for spawning subprocesses. Production code constructs the
// real bundle once at the top via [OSVenv]; tests construct an in-memory
// bundle and drive the full CLI through it.
//
// Downstream packages (for example internal/runner/run and internal/tflint)
// keep their own package-local Venv types so each owns its own contract.
// They convert from this root via a FromRoot constructor.
package venv

import (
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// Venv is the root virtualized environment. It carries the filesystem
// and process-execution handles that every Terragrunt operation needs.
type Venv struct {
	// FS backs every filesystem read and write.
	FS vfs.FS
	// Exec spawns every subprocess: tofu, terraform, git, hooks,
	// external auth providers, tflint.
	Exec vexec.Exec
}

// OSVenv builds the production [Venv]: the real OS filesystem and the
// real OS process executor.
func OSVenv() Venv {
	return Venv{FS: vfs.NewOSFS(), Exec: vexec.NewOSExec()}
}
