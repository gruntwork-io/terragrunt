//go:build http

package mcp_test

import (
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPDocsToolsRelayToTheHostedDocsServer pins that the docs server at
// mcp.docs.terragrunt.com lists tools and resources this server can relay,
// and answers a search and a resource read through it.
func TestHTTPDocsToolsRelayToTheHostedDocsServer(t *testing.T) {
	t.Parallel()

	session := newDocsSession(t, vhttp.NewOSClient(), func(*tgmcp.Options) {})

	names := docsToolNames(t, session)
	assert.Contains(t, names, "docs_search_docs")
	assert.Contains(t, names, "docs_read_doc_page")

	var out struct {
		Hits []map[string]any `json:"hits"`
	}

	callTool(t, session, "docs_search_docs", map[string]any{"query": "stacks"}, &out)

	assert.NotEmpty(t, out.Hits)

	uris := resourceURIs(t, session)
	require.NotEmpty(t, uris)

	res, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uris[0]})
	require.NoError(t, err)
	require.NotEmpty(t, res.Contents)
	assert.NotEmpty(t, res.Contents[0].Text)
}

// TestHTTPDocsToolCallRelaysTheHostedDocsServersRefusal pins that a call the
// hosted docs server refuses comes back as a tool error rather than failing
// the request.
func TestHTTPDocsToolCallRelaysTheHostedDocsServersRefusal(t *testing.T) {
	t.Parallel()

	session := newDocsSession(t, vhttp.NewOSClient(), func(*tgmcp.Options) {})

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "docs_lookup_reference",
		Arguments: map[string]any{},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError, "the hosted docs server accepted a lookup that names nothing")
	assert.NotEmpty(t, toolText(t, res))
}
