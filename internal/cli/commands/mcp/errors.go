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
