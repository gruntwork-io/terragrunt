package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// approvalRequestID names the one input request the mutating tools send. The
// client echoes it back as the key of the response map.
const approvalRequestID = "approval"

// approvalUnitsShown caps how many unit paths the approval prompt lists, so a
// large estate produces a prompt a person can still read.
const approvalUnitsShown = 40

// approvalEntryMaxLen caps one entry in the prompt, so a name the tree chose
// cannot add a paragraph of its own to the question.
const approvalEntryMaxLen = 120

// ErrApprovalDeclined is returned when the person asked to approve a run said
// no, or dismissed the prompt without answering. Both mean the run must not
// happen, and neither is a failure of the tool.
var ErrApprovalDeclined = errors.New(
	"the run was not approved, so nothing was applied or destroyed",
)

// ErrApprovalUnavailable is returned when the approval could not be put in
// front of a person at all, because the client does not implement
// elicitation. A run that nobody could approve does not proceed.
var ErrApprovalUnavailable = errors.New(
	"this MCP client cannot ask for approval (it does not support elicitation), and a run that nobody " +
		"can approve is refused; connect a client that supports elicitation, or run the command yourself",
)

// runMutatingCommand drives an apply or a destroy behind a human approval
// step. The first pass answers with an input request describing what the run
// would do. The client puts that in front of a person and calls again with
// their answer, and only an explicit acceptance reaches the run itself.
func runMutatingCommand(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	req *mcp.CallToolRequest,
	input runAllInput,
	command string,
) (*mcp.CallToolResult, runAllOutput, error) {
	answer, answered := approvalAnswer(req)

	if !answered {
		if !approvalReachesAPerson(req) {
			return nil, runAllOutput{}, ErrApprovalUnavailable
		}

		result, err := requestApproval(ctx, l, d, rootVenv, input, command)

		return result, runAllOutput{}, err
	}

	if answer == nil || answer.Action != "accept" {
		return nil, runAllOutput{}, ErrApprovalDeclined
	}

	out, err := runAllCommand(ctx, l, d, rootVenv, input, command)

	return nil, out, err
}

// approvalReachesAPerson reports whether this session can put an approval in
// front of someone. Both handshakes carry the client's capabilities, the
// legacy one through initialize and the newer one through the discover
// request, so a client with no elicitation support is known before anything is
// offered to it.
func approvalReachesAPerson(req *mcp.CallToolRequest) bool {
	params := req.Session.InitializeParams()

	return params != nil && params.Capabilities != nil && params.Capabilities.Elicitation != nil
}

// approvalAnswer returns the person's answer to the approval request, and
// whether this call is the retry that carries one. A retry whose response is
// not an elicitation result counts as answered but not accepted, so a
// malformed reply refuses the run rather than passing it through.
func approvalAnswer(req *mcp.CallToolRequest) (*mcp.ElicitResult, bool) {
	response, ok := req.Params.InputResponses[approvalRequestID]
	if !ok {
		return nil, false
	}

	result, ok := response.(*mcp.ElicitResult)
	if !ok {
		return nil, true
	}

	return result, true
}

// requestApproval builds the input request that asks a person to approve the
// run. The prompt names the units the run would touch, so what is approved is
// a specific change rather than the idea of a change.
func requestApproval(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	input runAllInput,
	command string,
) (*mcp.CallToolResult, error) {
	message, err := approvalMessage(ctx, l, d, rootVenv, input, command)
	if err != nil {
		return nil, err
	}

	return &mcp.CallToolResult{
		InputRequests: mcp.InputRequestMap{
			approvalRequestID: &mcp.ElicitParams{Message: message},
		},
	}, nil
}

// approvalMessage describes the pending run to the person approving it.
//
// It names the units and does not plan them. Planning here would cost a full
// run that the acceptance then repeats, and would not change the prompt, since
// a run summary reports per-unit outcomes and not a resource diff. Discovery
// answers the question `find --as=apply` and `find --as=destroy` answer, so the
// exclude blocks of the action being approved are honored.
func approvalMessage(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	input runAllInput,
	command string,
) (string, error) {
	order, err := runRunOrder(ctx, l, d, rootVenv, runOrderInput{
		WorkingDir: input.WorkingDir,
		Filter:     input.Filter,
		Command:    command,
	})
	if err != nil {
		return "", fmt.Errorf("describing the %s for approval: %w", command, err)
	}

	paths := make([]string, 0, len(order.Units))
	for _, unit := range order.Units {
		paths = append(paths, unit.Path)
	}

	b := &strings.Builder{}

	fmt.Fprintf(b, "Approve running `terragrunt run --all %s`?\n\n", command)

	if command == tf.CommandNameDestroy {
		approvalWriteUnits(b, "This DESTROYS the following units", paths)
		b.WriteString("\nDestroyed resources cannot be recovered, and any data in them is lost.\n")
	} else {
		approvalWriteUnits(b, "This APPLIES the following units", paths)
	}

	b.WriteString(
		"\nThis names the units, not the changes: nothing was planned to build this list. " +
			"Call the plan tool first to see what would change.\n",
	)

	fmt.Fprintf(
		b,
		"\nThe %s runs with -auto-approve, so accepting here is the only confirmation.\n",
		command,
	)

	return b.String(), nil
}

// approvalWriteUnits writes the unit list under a heading, capping it so a
// large estate still produces a readable prompt and saying how many were left
// out rather than silently truncating.
//
// Every entry is sanitized and bounded here, at the one place they reach a
// person. A path and a program name both come from the tree the agent chose,
// so an unsanitized one could open a line of its own and add reassurance the
// server never wrote to the question it is asking.
func approvalWriteUnits(b *strings.Builder, heading string, paths []string) {
	fmt.Fprintf(b, "%s (%d):\n", heading, len(paths))

	shown := paths
	if len(shown) > approvalUnitsShown {
		shown = shown[:approvalUnitsShown]
	}

	for _, path := range shown {
		fmt.Fprintf(b, "  - %s\n", truncateRunes(tui.SanitizeLabel(path), approvalEntryMaxLen))
	}

	if omitted := len(paths) - len(shown); omitted > 0 {
		fmt.Fprintf(b, "  ...and %d more\n", omitted)
	}
}
