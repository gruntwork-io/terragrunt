package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/version"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/venv"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	docsURL                   = "https://mcp.docs.terragrunt.com/mcp"
	docsClientName            = "terragrunt-mcp"
	docsToolPrefix            = "docs_"
	defaultDocsStartupTimeout = 5 * time.Second
	defaultDocsCallTimeout    = 30 * time.Second

	maxDocsResponseBytes = 4 << 20

	// MaxDocsTools is the most tools relayed from the docs server.
	MaxDocsTools = 64
	// MaxDocsResources is the most resources relayed from the docs server.
	MaxDocsResources = 256
	// MaxDocsListPages is the most pages read of each list the docs server
	// serves.
	MaxDocsListPages = 16

	// maxToolNameLen is the longest tool name the protocol allows.
	maxToolNameLen = 128

	// rejectedByTransport is the code of the error the SDK wraps around a
	// request its transport did not deliver.
	rejectedByTransport = -32005
)

var (
	errDocsResponseTooLarge = fmt.Errorf(
		"the Terragrunt docs server sent a response larger than %d bytes",
		maxDocsResponseBytes,
	)
	errUnservableDocsTool = errors.New(
		"the Terragrunt docs server listed a tool this server cannot relay",
	)
	errUnservableDocsResource = errors.New(
		"the Terragrunt docs server listed a resource this server cannot relay",
	)
)

// docsState is how far relaying the docs server got when this server started.
type docsState int

const (
	docsDenied docsState = iota
	docsUnreachable
	docsServed
)

// docsProxy relays the tools and resources the docs server listed when this
// server started, or tells a client why it relays none.
type docsProxy struct {
	relay   *docsRelay
	listing docsListing
	state   docsState
}

// docsListing is the instructions, tools, and resources the docs server listed
// when this server started.
type docsListing struct {
	instructions string
	tools        []*mcp.Tool
	resources    []*mcp.Resource
}

// docsRelay sends tool calls and resource reads on to the docs server.
type docsRelay struct {
	client      *mcp.Client
	httpClient  vhttp.Client
	userAgent   string
	callTimeout time.Duration
}

// docsTimeouts is how long each request to the docs server may take.
type docsTimeouts struct {
	startup time.Duration
	call    time.Duration
}

// startDocsProxy asks the docs server what it serves when granted includes
// [CapabilityDocs], waiting at most timeouts.startup. When asking fails, it
// logs a warning and returns a proxy that serves nothing.
func startDocsProxy(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	granted venv.Capabilities,
	timeouts docsTimeouts,
) *docsProxy {
	if !granted.Has(CapabilityDocs) {
		return &docsProxy{state: docsDenied}
	}

	v.RequireHTTP()

	relay := newDocsRelay(v.HTTP, timeouts.call)

	listing, err := relay.list(ctx, l, timeouts.startup)
	if err != nil {
		l.Warnf(
			"Not serving the Terragrunt docs tools, because the docs server at %s did not list them: %s",
			docsURL,
			tui.SanitizeLabel(describeDocsError(err)),
		)

		return &docsProxy{state: docsUnreachable}
	}

	return &docsProxy{relay: relay, listing: listing, state: docsServed}
}

// instructions describes the docs tools to a connecting client, and passes on
// the docs server's own instructions when it listed its tools.
func (p *docsProxy) instructions() string {
	if p.state == docsDenied {
		return "Tools relayed from the Terragrunt docs server are not served until the server is " +
			"restarted with --allow=docs."
	}

	if p.state == docsUnreachable {
		return "The server was started with --allow=docs, but the Terragrunt docs server did not " +
			"answer when this server started, so its tools are not served until the server is restarted."
	}

	served := "The server was started with --allow=docs, so each tool named " + docsToolPrefix +
		"* relays its call to the Terragrunt docs server. Arguments to those tools leave this " +
		"machine, so keep secrets out of them."

	if p.listing.instructions == "" {
		return served
	}

	return served + " The docs server's own instructions follow. They name its tools without the " +
		docsToolPrefix + " prefix.\n\n" + p.listing.instructions
}

// register adds the tools and resources the docs server listed to srv,
// skipping each one docsTool or docsResource refuses.
func (p *docsProxy) register(srv *mcp.Server, l log.Logger) {
	if p.state != docsServed {
		return
	}

	for _, remote := range p.listing.tools {
		tool, err := docsTool(remote)
		if err != nil {
			l.Warnf("Not serving a docs tool: %s", tui.SanitizeLabel(err.Error()))

			continue
		}

		srv.AddTool(tool, docsToolHandler(l, p.relay, remote.Name))
	}

	for _, remote := range p.listing.resources {
		resource, err := docsResource(remote)
		if err != nil {
			l.Warnf("Not serving a docs resource: %s", tui.SanitizeLabel(err.Error()))

			continue
		}

		srv.AddResource(resource, docsResourceHandler(l, p.relay))
	}
}

// newDocsRelay returns a relay to the docs server whose requests go through
// c, naming this server and its version in the User-Agent header and in the
// protocol handshake.
func newDocsRelay(c vhttp.Client, callTimeout time.Duration) *docsRelay {
	impl := &mcp.Implementation{Name: docsClientName, Version: version.GetVersion()}

	return &docsRelay{
		client:      mcp.NewClient(impl, nil),
		httpClient:  c,
		userAgent:   impl.Name + "/" + impl.Version,
		callTimeout: callTimeout,
	}
}

// list asks the docs server for its instructions and for the tools and
// resources it advertises, within timeout.
func (r *docsRelay) list(
	ctx context.Context,
	l log.Logger,
	timeout time.Duration,
) (docsListing, error) {
	return docsSession(
		ctx,
		l,
		r,
		timeout,
		func(ctx context.Context, s *mcp.ClientSession) (docsListing, error) {
			init := s.InitializeResult()
			caps := cmp.Or(init.Capabilities, &mcp.ServerCapabilities{})
			listing := docsListing{instructions: init.Instructions}

			if caps.Tools != nil {
				tools, err := listDocsPages(ctx, l, "tools", MaxDocsTools,
					func(ctx context.Context, cursor string) ([]*mcp.Tool, string, error) {
						res, err := s.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
						if err != nil {
							return nil, "", err
						}

						return res.Tools, res.NextCursor, nil
					})
				if err != nil {
					return docsListing{}, err
				}

				listing.tools = tools
			}

			if caps.Resources != nil {
				resources, err := listDocsPages(ctx, l, "resources", MaxDocsResources,
					func(ctx context.Context, cursor string) ([]*mcp.Resource, string, error) {
						res, err := s.ListResources(ctx, &mcp.ListResourcesParams{Cursor: cursor})
						if err != nil {
							return nil, "", err
						}

						return res.Resources, res.NextCursor, nil
					})
				if err != nil {
					return docsListing{}, err
				}

				listing.resources = resources
			}

			return listing, nil
		},
	)
}

// docsToolHandler relays a call to the docs server's tool named remoteName. A
// call that fails comes back as a tool error naming the cause.
func docsToolHandler(l log.Logger, r *docsRelay, remoteName string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := &mcp.CallToolParams{Name: remoteName}

		if len(req.Params.Arguments) > 0 {
			params.Arguments = req.Params.Arguments
		}

		res, err := docsSession(ctx, l, r, r.callTimeout,
			func(ctx context.Context, s *mcp.ClientSession) (*mcp.CallToolResult, error) {
				return s.CallTool(ctx, params)
			})
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{
					Text: "The call to the Terragrunt docs server failed: " + describeDocsError(
						err,
					),
				}},
			}, nil
		}

		return &mcp.CallToolResult{
			Content:           res.Content,
			StructuredContent: res.StructuredContent,
			IsError:           res.IsError,
		}, nil
	}
}

// docsResourceHandler relays a read of a resource the docs server listed.
func docsResourceHandler(l log.Logger, r *docsRelay) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		res, err := docsSession(ctx, l, r, r.callTimeout,
			func(ctx context.Context, s *mcp.ClientSession) (*mcp.ReadResourceResult, error) {
				return s.ReadResource(ctx, &mcp.ReadResourceParams{URI: req.Params.URI})
			})
		if err != nil {
			return nil, errors.New(
				"reading " + req.Params.URI + " from the Terragrunt docs server: " + describeDocsError(
					err,
				),
			)
		}

		return &mcp.ReadResourceResult{Contents: res.Contents}, nil
	}
}

// docsSession opens a session with the docs server, runs fn against it within
// timeout, and closes it.
func docsSession[T any](
	ctx context.Context,
	l log.Logger,
	r *docsRelay,
	timeout time.Duration,
	fn func(context.Context, *mcp.ClientSession) (T, error),
) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpClient := vhttp.WithTransport(r.httpClient, &docsTransport{
		base:      r.httpClient.Transport,
		session:   ctx,
		userAgent: r.userAgent,
	})

	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	session, err := r.client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             docsURL,
		HTTPClient:           httpClient,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		var zero T

		return zero, err
	}

	defer func() {
		if err := session.Close(); err != nil {
			l.Debugf(
				"Could not close the session with the Terragrunt docs server: %s",
				tui.SanitizeLabel(describeDocsError(err)),
			)
		}
	}()

	return fn(ctx, session)
}

// listDocsPages collects the items of a list the docs server pages through,
// keeping at most limit items from at most MaxDocsListPages pages. what names
// the items in the warning logged when either bound cuts the list short.
func listDocsPages[T any](
	ctx context.Context,
	l log.Logger,
	what string,
	limit int,
	page func(ctx context.Context, cursor string) ([]T, string, error),
) ([]T, error) {
	var items []T

	cursor := ""

	for range MaxDocsListPages {
		got, next, err := page(ctx, cursor)
		if err != nil {
			return nil, err
		}

		items = append(items, got...)

		if len(items) > limit {
			l.Warnf(
				"The Terragrunt docs server lists more than %d %s. Relaying the first %d.",
				limit,
				what,
				limit,
			)

			return items[:limit], nil
		}

		if next == "" {
			return items, nil
		}

		cursor = next
	}

	l.Warnf(
		"The Terragrunt docs server lists %s across more than %d pages. Relaying the first %d pages.",
		what,
		MaxDocsListPages,
		MaxDocsListPages,
	)

	return items, nil
}

// describeDocsError renders err from a request to the docs server. For a
// request that failed, it leaves out the address the request went to. For a
// refusal, it returns the message the docs server sent.
func describeDocsError(err error) string {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		return urlErr.Err.Error()
	}

	if rpcErr, ok := errors.AsType[*jsonrpc.Error](err); ok && rpcErr.Code != rejectedByTransport {
		return rpcErr.Message
	}

	return err.Error()
}

// docsTool returns the tool this server serves in place of remote, a tool the
// docs server lists. The name gains the docs_ prefix, icons and metadata are
// dropped, and the annotations say the tool only reads and reaches the
// network, whatever remote claims.
//
// It fails for a name the protocol does not allow, or an input schema that is
// not a JSON object of type "object".
func docsTool(remote *mcp.Tool) (*mcp.Tool, error) {
	if !validDocsToolName(remote.Name) {
		return nil, fmt.Errorf("%w: invalid name %q", errUnservableDocsTool, remote.Name)
	}

	if schema, ok := remote.InputSchema.(map[string]any); !ok || schema["type"] != "object" {
		return nil, fmt.Errorf(
			`%w: %s: input schema is not of type "object"`,
			errUnservableDocsTool,
			remote.Name,
		)
	}

	return &mcp.Tool{
		Name:         docsToolPrefix + remote.Name,
		Title:        remote.Title,
		Description:  remote.Description,
		InputSchema:  remote.InputSchema,
		OutputSchema: remote.OutputSchema,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(true),
		},
	}, nil
}

// validDocsToolName reports whether name, once it takes the docs_ prefix, is a
// tool name the protocol allows.
func validDocsToolName(name string) bool {
	return name != "" &&
		len(docsToolPrefix)+len(name) <= maxToolNameLen &&
		!strings.ContainsFunc(name, func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') &&
				r != '_' && r != '-' && r != '.'
		})
}

// docsResource returns the resource this server serves in place of remote, a
// resource the docs server lists, under the same URI. It fails for a URI that
// is not absolute.
func docsResource(remote *mcp.Resource) (*mcp.Resource, error) {
	if u, err := url.Parse(remote.URI); err != nil || u.Scheme == "" {
		return nil, fmt.Errorf(
			"%w: %q is not an absolute URI",
			errUnservableDocsResource,
			remote.URI,
		)
	}

	return &mcp.Resource{
		URI:         remote.URI,
		Name:        remote.Name,
		Title:       remote.Title,
		Description: remote.Description,
		MIMEType:    remote.MIMEType,
	}, nil
}

// docsTransport adds a User-Agent header naming this server to each request in
// one session with the docs server, refuses requests once the session has
// ended, and fails a response whose body runs past maxDocsResponseBytes.
type docsTransport struct {
	base      http.RoundTripper
	session   context.Context
	userAgent string
}

// RoundTrip implements [http.RoundTripper].
func (t *docsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.session.Err(); err != nil {
		return nil, err
	}

	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", t.userAgent)

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	resp.Body = &cappedBody{ReadCloser: resp.Body, remaining: maxDocsResponseBytes}

	return resp, nil
}

// cappedBody is a response body that fails with errDocsResponseTooLarge once
// more than remaining bytes have been read from it.
type cappedBody struct {
	io.ReadCloser
	remaining int64
}

// Read implements [io.Reader].
func (b *cappedBody) Read(p []byte) (int, error) {
	if room := b.remaining + 1; int64(len(p)) > room {
		p = p[:room]
	}

	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)

	if b.remaining < 0 {
		return n, errDocsResponseTooLarge
	}

	return n, err
}
