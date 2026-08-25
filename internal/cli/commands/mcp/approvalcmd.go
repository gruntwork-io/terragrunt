package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// commandApprovalRequestID names the input request that offers the programs a
// configuration wanted to run. The client echoes it back as the key of the
// response map.
const commandApprovalRequestID = "approve-commands"

// parseToolHandler adapts a parse-only tool's run function into an SDK
// handler that offers refused programs for approval before it answers.
func parseToolHandler[In, Out any](
	l log.Logger,
	d *serverDeps,
	name string,
	rootVenv *venv.Venv,
	run func(ctx context.Context, l log.Logger, d *serverDeps, rootVenv *venv.Venv, input In) (Out, error),
) mcp.ToolHandlerFor[In, Out] {
	return multiRoundTripToolHandler(
		l,
		name,
		func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
			return runWithCommandApproval(
				ctx,
				d,
				req,
				func(ctx context.Context, call *serverDeps) (Out, error) {
					return run(ctx, l, call, rootVenv, input)
				},
			)
		},
	)
}

// runWithCommandApproval runs a parse-only tool and, when the configuration
// asked for programs the allowlist refused, offers them to the person
// operating the client rather than quietly returning a result built without
// them. Accepting runs the tool again with those programs allowed.
//
// The offer is made once, so the tool runs at most twice however deep the
// configuration nests its programs. A program only reached because an
// approved one returned output stays refused and is reported in the result's
// degraded list.
//
// The SDK discards out alongside an error or an input request, so the exits
// below leave whatever run returned in place.
func runWithCommandApproval[Out any](
	ctx context.Context,
	d *serverDeps,
	req *mcp.CallToolRequest,
	run func(ctx context.Context, d *serverDeps) (Out, error),
) (result *mcp.CallToolResult, out Out, err error) {
	approved, answered := commandApprovalAnswer(req)

	call := d.forCall(approved)

	out, err = run(ctx, call)
	if err != nil {
		return nil, out, err
	}

	// A client with no way to ask keeps the answer it would have had before
	// this offer existed: the result with those programs stubbed, and a
	// degraded list saying so. These tools only read, so degrading beats
	// failing a call nobody could have approved.
	refused := call.rec.refusedCommands()
	if len(refused) == 0 || answered || !approvalReachesAPerson(req) {
		return nil, out, nil
	}

	state, err := json.Marshal(refused)
	if err != nil {
		return nil, out, fmt.Errorf("recording the programs offered for approval: %w", err)
	}

	result = &mcp.CallToolResult{
		RequestState: string(state),
		InputRequests: mcp.InputRequestMap{
			commandApprovalRequestID: &mcp.ElicitParams{Message: commandApprovalMessage(refused)},
		},
	}

	return result, out, nil
}

// commandApprovalAnswer returns the programs the person approved, and whether
// this call is the retry that carries an answer at all. A decline answers with
// nothing approved, which is not an error: the tool still returns the result
// it can build with those programs stubbed.
//
// The approved set comes from the request state the client echoed back rather
// than from a fresh parse. A client that tampers with it could name a program
// nobody offered, but the same client draws the prompt, so it could as easily
// have accepted one it never showed. The state adds no authority it lacks.
func commandApprovalAnswer(req *mcp.CallToolRequest) ([]string, bool) {
	response, ok := req.Params.InputResponses[commandApprovalRequestID]
	if !ok {
		return nil, false
	}

	result, ok := response.(*mcp.ElicitResult)
	if !ok || result.Action != "accept" {
		return nil, true
	}

	var approved []string
	if err := json.Unmarshal([]byte(req.Params.RequestState), &approved); err != nil {
		return nil, true
	}

	return approved, true
}

// commandApprovalMessage describes the programs to the person approving them.
// It names them and nothing else, because the arguments differ per unit and
// the decision is about the program.
func commandApprovalMessage(refused []string) string {
	b := &strings.Builder{}

	b.WriteString(
		"This Terragrunt configuration runs programs of its own, through run_cmd(), a hook, or " +
			"an auth-provider command. They did not run.\n\n",
	)

	approvalWriteUnits(b, "Approve running", refused)

	b.WriteString(
		"\nThese come from the configuration under the server root, not from the agent. " +
			"Approving runs them on this machine with the credentials this server holds. " +
			"Declining returns the result with each of them substituted by an empty string.\n",
	)

	return b.String()
}
