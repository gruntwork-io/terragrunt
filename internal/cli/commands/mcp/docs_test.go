package mcp_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/version"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	docsPatternURI = "pattern://scoped-run-all"
	docsMissingURI = "pattern://missing"
	docsRefusal    = "lookup_reference matches names, not free text"
)

type docsSearchInput struct {
	Query string `json:"query"`
}

type docsSearchOutput struct {
	Hits []string `json:"hits"`
}

type docsPageInput struct {
	Size int `json:"size"`
}

// docsServer is a docs MCP server that answers every request its client sends,
// whatever the host, the way the hosted one does: statelessly, with JSON
// bodies.
type docsServer struct {
	client     vhttp.Client
	userAgents []string
	clients    []any
	mu         sync.Mutex
	down       atomic.Bool
}

// newDocsServer serves a search_docs tool, a read_doc_page tool returning
// size bytes of text, a lookup_reference tool that refuses every call with a
// protocol error, a readable resource, and a resource that fails to read.
// configure can change the server before it answers.
func newDocsServer(t *testing.T, configure func(*mcp.Server)) *docsServer {
	t.Helper()

	return newDocsServerWithPageSize(t, 0, configure)
}

// newDocsServerWithPageSize is [newDocsServer] with each list the server
// serves cut into pages of pageSize items.
func newDocsServerWithPageSize(
	t *testing.T,
	pageSize int,
	configure func(*mcp.Server),
) *docsServer {
	t.Helper()

	docs := &docsServer{}

	srv := mcp.NewServer(
		&mcp.Implementation{Name: "docs-test", Version: "1"},
		&mcp.ServerOptions{Instructions: "Search before you read.", PageSize: pageSize},
	)

	mcp.AddTool(
		srv,
		&mcp.Tool{Name: "search_docs", Description: "Search the docs."},
		func(_ context.Context, req *mcp.CallToolRequest, in docsSearchInput) (*mcp.CallToolResult, docsSearchOutput, error) {
			info, ok := req.Params.Meta[mcp.MetaKeyClientInfo].(map[string]any)
			assert.True(t, ok, "the call carried no client information")

			docs.mu.Lock()
			docs.clients = append(docs.clients, info["name"])
			docs.mu.Unlock()

			return nil, docsSearchOutput{Hits: []string{"hit for " + in.Query}}, nil
		},
	)

	mcp.AddTool(
		srv,
		&mcp.Tool{Name: "read_doc_page", Description: "Read a page."},
		func(_ context.Context, _ *mcp.CallToolRequest, in docsPageInput) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat("a", in.Size)}},
			}, nil, nil
		},
	)

	srv.AddTool(
		&mcp.Tool{Name: "lookup_reference", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: docsRefusal}
		},
	)

	srv.AddResource(
		&mcp.Resource{URI: docsPatternURI, Name: "scoped-run-all", MIMEType: "text/markdown"},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: "text/markdown", Text: "Scope the run."},
			}}, nil
		},
	)

	srv.AddResource(
		&mcp.Resource{URI: docsMissingURI, Name: "missing", MIMEType: "text/markdown"},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		},
	)

	configure(srv)

	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)

	docs.client = vhttp.NewMemClient(
		func(_ context.Context, req *http.Request) (*http.Response, error) {
			docs.mu.Lock()
			docs.userAgents = append(docs.userAgents, req.UserAgent())
			docs.mu.Unlock()

			if docs.down.Load() {
				return vhttp.Respond(http.StatusInternalServerError, nil, nil), nil
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			return rec.Result(), nil
		},
	)

	return docs
}

func (d *docsServer) seenUserAgents() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.userAgents)
}

func (d *docsServer) seenClients() []any {
	d.mu.Lock()
	defer d.mu.Unlock()

	return slices.Clone(d.clients)
}

// newDocsSession serves the tool set with --allow=docs granted, relaying to
// the docs server through the client c. configure can change the options
// further.
func newDocsSession(
	t *testing.T,
	c vhttp.Client,
	configure func(*tgmcp.Options),
) *mcp.ClientSession {
	t.Helper()

	return newTestSessionWith(
		t,
		newTestTree(t),
		venvtest.NewWithOSFS().WithHTTP(c),
		func(o *tgmcp.Options) {
			o.Allow = []string{"docs"}
			configure(o)
		},
		nil,
	)
}

func docsToolNames(t *testing.T, session *mcp.ClientSession) []string {
	t.Helper()

	return slices.DeleteFunc(toolNames(t, session), func(name string) bool {
		return !strings.HasPrefix(name, "docs_")
	})
}

func resourceURIs(t *testing.T, session *mcp.ClientSession) []string {
	t.Helper()

	listed, err := session.ListResources(t.Context(), nil)
	require.NoError(t, err)

	uris := make([]string, 0, len(listed.Resources))
	for _, resource := range listed.Resources {
		uris = append(uris, resource.URI)
	}

	return uris
}

// addDocsTools adds n tools named extra_<i> to srv, each answering with its
// own name.
func addDocsTools(srv *mcp.Server, n int) {
	for i := range n {
		name := fmt.Sprintf("extra_%03d", i)

		srv.AddTool(
			&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}},
			func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: name}},
				}, nil
			},
		)
	}
}

// relayingExcept returns a client that relays every request to docs, except a
// call of method, which it holds until the request ends.
func relayingExcept(docs *docsServer, method string) vhttp.Client {
	return vhttp.NewMemClient(
		func(ctx context.Context, req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}

			if bytes.Contains(body, []byte(`"`+method+`"`)) {
				<-ctx.Done()

				return nil, ctx.Err()
			}

			req.Body = io.NopCloser(bytes.NewReader(body))

			return docs.client.Transport.RoundTrip(req)
		},
	)
}

func toolText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()

	require.Len(t, res.Content, 1)

	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok, "the tool answered with %T, not text", res.Content[0])

	return text.Text
}

func TestDocsToolsRelayToTheDocsServer(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(*mcp.Server) {})
	session := newDocsSession(t, docs.client, func(*tgmcp.Options) {})

	assert.Equal(
		t,
		[]string{"docs_lookup_reference", "docs_read_doc_page", "docs_search_docs"},
		docsToolNames(t, session),
	)
	assert.Contains(t, toolNames(t, session), "discover")
	assert.Contains(t, session.InitializeResult().Instructions, "Search before you read.")

	var out docsSearchOutput

	callTool(t, session, "docs_search_docs", map[string]any{"query": "stacks"}, &out)

	assert.Equal(t, []string{"hit for stacks"}, out.Hits)
}

// TestDocsRequestsIdentifyTerragrunt pins that every request to the docs
// server names Terragrunt's MCP server and its version in the User-Agent
// header, and that each call names it as the client.
func TestDocsRequestsIdentifyTerragrunt(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(*mcp.Server) {})
	session := newDocsSession(t, docs.client, func(*tgmcp.Options) {})

	var out docsSearchOutput

	callTool(t, session, "docs_search_docs", map[string]any{"query": "stacks"}, &out)

	userAgents := docs.seenUserAgents()
	require.NotEmpty(t, userAgents)

	for _, ua := range userAgents {
		assert.Equal(t, "terragrunt-mcp/"+version.GetVersion(), ua)
	}

	assert.Equal(t, []any{"terragrunt-mcp"}, docs.seenClients())
}

func TestDocsResourcesRelayToTheDocsServer(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(*mcp.Server) {})
	session := newDocsSession(t, docs.client, func(*tgmcp.Options) {})

	assert.ElementsMatch(t, []string{docsPatternURI, docsMissingURI}, resourceURIs(t, session))

	res, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: docsPatternURI})
	require.NoError(t, err)
	require.Len(t, res.Contents, 1)
	assert.Equal(t, "Scope the run.", res.Contents[0].Text)
}

// TestDocsResourceReadFailuresAreRelayed pins that a resource the docs server
// lists but cannot read fails to read through this server too.
func TestDocsResourceReadFailuresAreRelayed(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(*mcp.Server) {})
	session := newDocsSession(t, docs.client, func(*tgmcp.Options) {})

	_, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: docsMissingURI})
	require.Error(t, err)

	res, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: docsPatternURI})
	require.NoError(t, err)
	require.Len(t, res.Contents, 1)
	assert.Equal(t, "Scope the run.", res.Contents[0].Text)
}

func TestDocsToolsAreAbsentWithoutAllowDocs(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(*mcp.Server) {})
	session := newDocsSession(t, docs.client, func(o *tgmcp.Options) {
		o.Allow = nil
	})

	assert.Empty(t, docsToolNames(t, session))
	assert.Empty(t, docs.seenUserAgents(), "a request reached the docs server")
}

func TestServeStartsWithoutDocsWhenTheDocsServerIsDown(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(*mcp.Server) {})
	docs.down.Store(true)

	session := newDocsSession(t, docs.client, func(*tgmcp.Options) {})

	assert.Empty(t, docsToolNames(t, session))
	assert.Contains(t, toolNames(t, session), "discover")
}

// TestServeStartsWithoutDocsWhenTheDocsServerIsSlow pins that a docs server
// that never answers holds up startup only until the startup timeout.
func TestServeStartsWithoutDocsWhenTheDocsServerIsSlow(t *testing.T) {
	t.Parallel()

	slow := vhttp.NewMemClient(func(ctx context.Context, _ *http.Request) (*http.Response, error) {
		<-ctx.Done()

		return nil, ctx.Err()
	})

	start := time.Now()

	session := newDocsSession(t, slow, func(o *tgmcp.Options) {
		o.DocsStartupTimeout = 50 * time.Millisecond
	})

	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Empty(t, docsToolNames(t, session))
	assert.Contains(t, toolNames(t, session), "discover")
}

// TestDocsToolCallThatHangsEndsAtTheCallTimeout pins that a docs server that
// stops answering mid-call holds the caller only until the call timeout, and
// that the call comes back as a tool error.
func TestDocsToolCallThatHangsEndsAtTheCallTimeout(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(*mcp.Server) {})

	session := newDocsSession(t, relayingExcept(docs, "tools/call"), func(o *tgmcp.Options) {
		o.DocsCallTimeout = 50 * time.Millisecond
	})

	start := time.Now()

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "docs_search_docs",
		Arguments: map[string]any{"query": "stacks"},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Less(t, time.Since(start), 2*time.Second)
}

// TestDocsSessionSendsNothingAfterItEnds pins that a session that runs out of
// time does not go on to end itself on the docs server.
func TestDocsSessionSendsNothingAfterItEnds(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(*mcp.Server) {})

	var deletes atomic.Int32

	stateful := vhttp.NewMemClient(
		func(ctx context.Context, req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodDelete {
				deletes.Add(1)

				return vhttp.Respond(http.StatusNoContent, nil, nil), nil
			}

			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}

			if bytes.Contains(body, []byte(`"tools/list"`)) {
				<-ctx.Done()

				return nil, ctx.Err()
			}

			req.Body = io.NopCloser(bytes.NewReader(body))

			resp, err := docs.client.Transport.RoundTrip(req)
			if err != nil {
				return nil, err
			}

			resp.Header.Set("Mcp-Session-Id", "docs-session")

			return resp, nil
		},
	)

	session := newDocsSession(t, stateful, func(o *tgmcp.Options) {
		o.DocsStartupTimeout = 50 * time.Millisecond
	})

	assert.Empty(t, docsToolNames(t, session))
	assert.Zero(t, deletes.Load(), "a request went out after the session ended")
}

func TestDocsServerRedirectsAreNotFollowed(t *testing.T) {
	t.Parallel()

	const elsewhere = "elsewhere.example"

	var redirected, followed atomic.Bool

	redirecting := vhttp.NewMemClient(
		func(_ context.Context, req *http.Request) (*http.Response, error) {
			if req.URL.Host == elsewhere {
				followed.Store(true)

				return vhttp.Respond(http.StatusNotFound, nil, nil), nil
			}

			redirected.Store(true)

			return vhttp.Respond(http.StatusTemporaryRedirect, nil, http.Header{
				"Location": []string{"https://" + elsewhere + "/mcp"},
			}), nil
		},
	)

	session := newDocsSession(t, redirecting, func(*tgmcp.Options) {})

	assert.Empty(t, docsToolNames(t, session))
	assert.True(t, redirected.Load(), "no request reached the docs server")
	assert.False(t, followed.Load(), "a request followed the redirect")
}

// TestDocsToolCallsSurviveAnOutage pins that a call made while the docs
// server fails comes back as a tool error, and that the next call reaches the
// docs server again once it recovers.
func TestDocsToolCallsSurviveAnOutage(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(*mcp.Server) {})
	session := newDocsSession(t, docs.client, func(*tgmcp.Options) {})

	docs.down.Store(true)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "docs_search_docs",
		Arguments: map[string]any{"query": "stacks"},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)

	docs.down.Store(false)

	var out docsSearchOutput

	callTool(t, session, "docs_search_docs", map[string]any{"query": "stacks"}, &out)

	assert.Equal(t, []string{"hit for stacks"}, out.Hits)
}

// TestDocsToolCallRelaysTheDocsServersRefusal pins that a call the docs
// server answers with a protocol error, as the hosted one does for arguments
// it rejects, comes back as a tool error carrying the docs server's message.
func TestDocsToolCallRelaysTheDocsServersRefusal(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(*mcp.Server) {})
	session := newDocsSession(t, docs.client, func(*tgmcp.Options) {})

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "docs_lookup_reference",
		Arguments: map[string]any{},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Contains(t, toolText(t, res), docsRefusal)
}

// TestDocsToolsBeyondTheCapAreNotRelayed pins that a docs server listing more
// tools than [tgmcp.MaxDocsTools] gets that many relayed and no more.
func TestDocsToolsBeyondTheCapAreNotRelayed(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(srv *mcp.Server) {
		addDocsTools(srv, tgmcp.MaxDocsTools+5)
	})
	session := newDocsSession(t, docs.client, func(*tgmcp.Options) {})

	assert.Len(t, docsToolNames(t, session), tgmcp.MaxDocsTools)
}

// TestDocsToolPagesBeyondTheCapAreNotRelayed pins that a docs server paging
// its tools across more than [tgmcp.MaxDocsListPages] pages gets that many
// pages relayed and no more.
func TestDocsToolPagesBeyondTheCapAreNotRelayed(t *testing.T) {
	t.Parallel()

	docs := newDocsServerWithPageSize(t, 1, func(srv *mcp.Server) {
		addDocsTools(srv, tgmcp.MaxDocsListPages+5)
	})
	session := newDocsSession(t, docs.client, func(*tgmcp.Options) {})

	assert.Len(t, docsToolNames(t, session), tgmcp.MaxDocsListPages)
}

func TestDocsToolCallRefusesAnOversizedResponse(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(*mcp.Server) {})
	session := newDocsSession(t, docs.client, func(*tgmcp.Options) {})

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "docs_read_doc_page",
		Arguments: map[string]any{"size": 5 << 20},
	})
	require.NoError(t, err)
	assert.True(t, res.IsError)

	res, err = session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "docs_read_doc_page",
		Arguments: map[string]any{"size": 1 << 10},
	})
	require.NoError(t, err)
	assert.False(t, res.IsError)
}

// TestDocsDefinitionsTheServerCannotRelayAreSkipped pins that a tool or
// resource the local server cannot serve is left out, and that everything
// else the docs server lists is still served.
func TestDocsDefinitionsTheServerCannotRelayAreSkipped(t *testing.T) {
	t.Parallel()

	docs := newDocsServer(t, func(srv *mcp.Server) {
		srv.AddReceivingMiddleware(addUnrelayableDefinitions)
	})

	session := newDocsSession(t, docs.client, func(*tgmcp.Options) {})

	assert.Equal(
		t,
		[]string{"docs_lookup_reference", "docs_read_doc_page", "docs_search_docs"},
		docsToolNames(t, session),
	)
	assert.ElementsMatch(t, []string{docsPatternURI, docsMissingURI}, resourceURIs(t, session))
}

// addUnrelayableDefinitions is middleware that adds definitions the proxy
// refuses to the tools and resources a docs server lists.
func addUnrelayableDefinitions(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		res, err := next(ctx, method, req)

		switch listed := res.(type) {
		case *mcp.ListToolsResult:
			listed.Tools = append(listed.Tools, unrelayableTools()...)
		case *mcp.ListResourcesResult:
			listed.Resources = append(listed.Resources, &mcp.Resource{URI: "no-scheme", Name: "relative"})
		}

		return res, err
	}
}

func unrelayableTools() []*mcp.Tool {
	object := map[string]any{"type": "object"}
	headerInput := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "object", "x-mcp-header": "Not A Header"},
		},
	}

	return []*mcp.Tool{
		{Name: "has space", InputSchema: object},
		{Name: strings.Repeat("a", 128), InputSchema: object},
		{Name: "array_input", InputSchema: map[string]any{"type": "array"}},
		{Name: "header_input", InputSchema: headerInput},
	}
}
