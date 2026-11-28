package mcp

import (
	"errors"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/venv"
)

// ErrApplyRequiresExec is returned when the apply and destroy tools are asked
// for without the capability to start the processes an apply is made of.
// Asking for both flags is deliberate. Granting execution as a side effect of
// a flag whose name says nothing about it would be a surprise on the one path
// where surprises are expensive.
var ErrApplyRequiresExec = errors.New(
	"--" + AllowApplyFlagName + " requires --" + AllowFlagName + "=" + string(
		venv.CapabilityExec,
	) + ": applying is running tofu",
)

// ErrApplyRequiresFSRW is returned when the apply and destroy tools are asked
// for without the capability to write the directories tofu runs in.
var ErrApplyRequiresFSRW = errors.New(
	"--" + AllowApplyFlagName + " requires --" + AllowFlagName + "=" + string(
		venv.CapabilityFSRW,
	) + ": tofu writes to the directory it runs in",
)

// ErrFmtRequiresCheck is returned when the fmt tool is asked to rewrite files
// on a server that cannot write them.
var ErrFmtRequiresCheck = errors.New(
	"this server cannot write files: pass check=true to report the files that need formatting, " +
		"or restart the MCP server with --" + AllowFlagName + "=" + string(venv.CapabilityFSRW) + " to rewrite them",
)

// ErrCatalogCloneRequiresFSRW is the error the catalog tool reports for a
// remote repository on a server that cannot write the clone to disk.
var ErrCatalogCloneRequiresFSRW = errors.New(
	"cloning a remote repository writes it to disk: restart the MCP server with --" +
		AllowFlagName + "=" + string(venv.CapabilityExec) + " and --" +
		AllowFlagName + "=" + string(venv.CapabilityFSRW),
)

// ErrGitFilterRequiresExecAndFSRW is returned when a filter query names a git
// range on a server that cannot check the range out.
var ErrGitFilterRequiresExecAndFSRW = errors.New(
	"git-range filters check each ref out with git: restart the MCP server with --" +
		AllowFlagName + "=" + string(venv.CapabilityExec) + " and --" +
		AllowFlagName + "=" + string(venv.CapabilityFSRW),
)

// ErrExperimentRequired is returned when the mcp command is run without the
// mcp-command experiment enabled.
var ErrExperimentRequired = errors.New(
	"the '" + CommandName + "' command requires the '" + experiment.MCPCommand + "' experiment to be enabled" +
		" (set --experiment " + experiment.MCPCommand + " or TG_EXPERIMENT=" + experiment.MCPCommand + ")",
)

// ErrOutsideRoot is returned when a path a tool-call argument names resolves
// outside the MCP server root.
var ErrOutsideRoot = errors.New("escapes the MCP server root")

// ErrScaffoldTooManyEntries is returned when a downloaded source, or what
// scaffolding generated from it, has more entries than the tool walks.
var ErrScaffoldTooManyEntries = fmt.Errorf(
	"the source has more than %d files and directories", scaffoldMaxEntries,
)

// ErrScaffoldUnsafeTemplate is returned when a source or template could write
// outside the output directory or read outside the source once rendered.
var ErrScaffoldUnsafeTemplate = errors.New("the scaffold tool does not render this source")

// ErrScaffoldSourceRequired is returned when a call names no source.
var ErrScaffoldSourceRequired = errors.New("source is required")

// ErrScaffoldVarName is returned when a vars name is empty or holds '='.
var ErrScaffoldVarName = errors.New("a vars name must be non-empty and cannot contain '='")

// ErrScaffoldValueNotHCL is returned when a values entry does not parse as an
// HCL expression.
var ErrScaffoldValueNotHCL = errors.New("not an HCL expression")

// ErrScaffoldVarFileNotFile is returned when a var_files entry is not a
// regular file.
var ErrScaffoldVarFileNotFile = errors.New("not a file")

// ErrScaffoldOutputFolderNotDir is returned when output_folder exists and is
// not a directory.
var ErrScaffoldOutputFolderNotDir = errors.New("not a directory")

// ErrScaffoldOutputFolderNotEmpty is returned when output_folder holds
// anything.
var ErrScaffoldOutputFolderNotEmpty = errors.New(
	"not empty; scaffold into a new or empty directory so nothing is overwritten",
)

// ErrScaffoldNotAModule is returned when template, vars, or var_files are
// given for a unit or stack.
var ErrScaffoldNotAModule = errors.New(
	"template, vars, and var_files apply only to a module; a unit or stack is copied as it is written, " +
		"so set what it needs through values",
)

// ErrScaffoldUnknownValue is returned when values sets a name the source does
// not take.
var ErrScaffoldUnknownValue = errors.New("values sets a name the source does not take")
