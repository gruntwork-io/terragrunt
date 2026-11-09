// Package venv exposes Terragrunt's virtualized environment to programs that
// embed Terragrunt, such as a language server calling pkg/config. A [Venv]
// carries every handle Terragrunt reaches the system through: the filesystem,
// process execution, outbound HTTP, SOPS decryption, environment variables,
// and the console.
//
// Build one of three ways. [OSVenv] reaches the real system. [MemVenv] reaches
// nothing outside the process, for fast tests. [Restrict] narrows another
// Venv to the capabilities a caller grants, for evaluating configurations it
// does not trust.
package venv

import ivenv "github.com/gruntwork-io/terragrunt/internal/venv"

// Venv is the environment pkg/config parsing functions and Terragrunt runs
// take. Derive variants with its With methods, which return copies.
type Venv = ivenv.Venv

// OSVenv returns a [Venv] backed by the real filesystem, process executor,
// HTTP client, and SOPS decrypter, a snapshot of the process environment, and
// the process's stdin, stdout, and stderr.
func OSVenv() *Venv {
	return ivenv.OSVenv()
}

// MemVenv returns a [Venv] that reaches nothing outside the process: an
// in-memory filesystem, an executor that fails every command, no network, an
// empty environment, and writers that discard. Seed its filesystem through
// its FS field, and answer commands with [Venv.WithHandler].
func MemVenv() *Venv {
	return ivenv.MemVenv()
}
