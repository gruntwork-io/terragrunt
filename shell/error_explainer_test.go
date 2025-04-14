package shell_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
)

func TestExplainError(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		errorOutput string
		explanation string
	}{
		{
			errorOutput: "Error refreshing state: AccessDenied: Access Denied",
			explanation: "Check your credentials and permissions",
		},
		{
			errorOutput: "Error: Initialization required",
			explanation: "You need to run terragrunt (run-all) init to initialize working directory",
		},
		{
			errorOutput: "Module source has changed",
			explanation: "You need to run terragrunt (run-all) init install all required modules",
		},
		{
			errorOutput: "Error: Failed to get existing workspaces: Unable to list objects in S3 bucket \"mybucket\": operation error S3: ListObjectsV2, https response error StatusCode: 301, RequestID: GH67DSB7KB8H578N, HostID: vofohiXBwNhR8Im+Dj7RpUPCPnOq9IDfn1rsUHHCzN9HgVMFfuIH5epndgLQvDeJPz2DrlUh0tA=, requested bucket from \"us-east-1\", actual location \"eu-west-1\"\n",
			explanation: "You don't have access to the S3 bucket where the state is stored. Check your credentials and permissions.",
		},
		{
			errorOutput: "exec: \"tofu\": executable file not found in $PATH",
			explanation: "The executables 'terraform' and 'tofu' are missing from your $PATH. Please add at least one of these to your $PATH.",
		},
		{
			errorOutput: "Error: Reference to undeclared input variable   on main.tf line 12, in resource \"aws_s3_bucket\" \"example\":   12:   bucket = var.values.bucket_name  An input variable with the name \"values\" has not been declared. This variable can be declared with a variable \"values\" {} block.╵",
			explanation: "You are using a stacks feature without enabling it. Enable the stacks experiment through CLI flag \"--experiment stacks\"",
		},
		{
			errorOutput: "Error: There is no variable named \"values\"",
			explanation: "You are using a stacks feature without enabling it. Enable the stacks experiment through CLI flag \"--experiment stacks\"",
		},
		{
			errorOutput: "Error: The input variable \"values\" is not declared in the root module on variables.tf line 3:    3:   default     = var.values.environment  Input variables can only be referenced from the same module where they are declared.╵",
			explanation: "You are using a stacks feature without enabling it. Enable the stacks experiment through CLI flag \"--experiment stacks\"",
		},
		{
			errorOutput: " Error: Reference to undeclared input variable   on services/main.tf line 5, in module \"app\":    5:   environment = var.values.environment An input variable with the name \"values\" has not been declared.",
			explanation: "You are using a stacks feature without enabling it. Enable the stacks experiment through CLI flag \"--experiment stacks\"",
		},
	}

	for _, tt := range testCases {
		tt := tt

		t.Run(tt.errorOutput, func(t *testing.T) {
			t.Parallel()

			output := util.CmdOutput{}
			output.Stderr = *bytes.NewBufferString(tt.errorOutput)

			errs := new(errors.MultiError)
			errs = errs.Append(util.ProcessExecutionError{
				Err:    errors.New(""),
				Output: output,
			})
			explanation := shell.ExplainError(errs)
			assert.Contains(t, explanation, tt.explanation)
		})
	}
}
