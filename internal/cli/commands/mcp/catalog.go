package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/redact"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// catalogLimitMax caps the components one catalog call returns. The
	// repositories a catalog names decide how many components there are.
	catalogLimitMax = 500

	// catalogDocMaxRunes caps the README body returned for one component.
	catalogDocMaxRunes = 16_000

	// catalogFailureMaxRunes caps the error reported for one repository, so
	// the output of a failed clone cannot balloon the result.
	catalogFailureMaxRunes = 1000
)

type catalogInput struct {
	WorkingDir  string `json:"working_dir,omitempty"  jsonschema:"Directory to read the catalog configuration from, relative to the server root. Defaults to the server root."`
	Limit       int    `json:"limit,omitempty"        jsonschema:"Most components to return. The default and maximum is 500."`
	IncludeDocs bool   `json:"include_docs,omitempty" jsonschema:"Return each component's README body in 'doc', cut at 16000 characters."`
}

type catalogFailure struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type catalogOutput struct {
	Components     []*format.Entry  `json:"components"`
	Failures       []catalogFailure `json:"failures,omitempty"`
	Degraded       []string         `json:"degraded,omitempty"`
	ComponentCount int              `json:"component_count"`
}

// catalogDocs selects whether a returned component keeps its README body.
type catalogDocs int

const (
	catalogOmitDocs catalogDocs = iota
	catalogIncludeDocs
)

// catalogDocsFor maps the tool-call argument onto whether README bodies come
// back.
func catalogDocsFor(includeDocs bool) catalogDocs {
	if includeDocs {
		return catalogIncludeDocs
	}

	return catalogOmitDocs
}

func registerCatalog(srv *mcp.Server, l log.Logger, d *serverDeps, rootVenv *venv.Venv) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "catalog",
		Description: "List the modules, templates, units, and stacks in the catalog the configuration names: the " +
			"repositories in the root configuration's catalog block, plus those the units' terraform sources " +
			"point at. Each component comes back in the shape `terragrunt catalog --format=jsonl` prints, and " +
			"'component_source' is the value to put in a terraform block's source. 'component_count' counts " +
			"every component found, even past 'limit'. Set include_docs=true only when you need the READMEs; " +
			"they are most of the result's size. A repository that fails to load is listed in 'failures'. " +
			"Cloning a remote repository runs git, which needs --allow=exec; a local directory needs nothing.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  new(d.reachesNetwork()),
		},
	}, parseToolHandler(l, d, "catalog", rootVenv, runCatalog))
}

func runCatalog(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	input catalogInput,
) (catalogOutput, error) {
	limit, err := catalogLimit(input.Limit)
	if err != nil {
		return catalogOutput{}, err
	}

	dir, err := resolveWorkingDir(rootVenv.FS, d.launchDir, input.WorkingDir)
	if err != nil {
		return catalogOutput{}, err
	}

	opts, env, err := buildDirOptions(l, d, rootVenv, dir, nil)
	if err != nil {
		return catalogOutput{}, err
	}

	cv := d.callVenv(rootVenv, env, io.Discard)

	ctx = freshCallContext(ctx)

	// The catalog block is read from the first file named ScaffoldRootFileName
	// in the directories above TerragruntConfigPath.
	opts.TerragruntConfigPath = filepath.Join(dir, config.DefaultTerragruntConfigPath)
	opts.ScaffoldRootFileName = scaffold.GetDefaultRootFileName(ctx, cv.FS, opts)

	tempDirs := tui.NewTempDirTracker(cv.FS)
	defer tempDirs.Cleanup(l)

	collected := &catalogCollector{
		entries: []*format.Entry{},
		limit:   limit,
		docs:    catalogDocsFor(input.IncludeDocs),
	}

	catalogOpts := catalog.NewOptions(opts)
	catalogOpts.UnreadableConfig = catalog.ReportUnreadableConfig

	streamErr := catalog.Stream(
		ctx, l, io.Discard, collected,
		catalog.NewLoadFunc(l, cv, catalogOpts, tempDirs, ""),
	)

	failures, err := catalogFailures(streamErr)
	if err != nil {
		return catalogOutput{}, err
	}

	return catalogOutput{
		Components:     collected.entries,
		Failures:       failures,
		Degraded:       d.rec.notes(),
		ComponentCount: collected.found,
	}, nil
}

// catalogLimit maps the tool-call limit argument onto the most components a
// call returns. Zero means [catalogLimitMax].
func catalogLimit(requested int) (int, error) {
	if requested == 0 {
		return catalogLimitMax, nil
	}

	if requested < 0 || requested > catalogLimitMax {
		return 0, fmt.Errorf("limit %d is out of range: expected 1 to %d", requested, catalogLimitMax)
	}

	return requested, nil
}

// catalogFailures turns the repositories a load could not read into result
// entries, so the components the other repositories gave still come back. Any
// other error fails the call.
//
// Each repository is named as the configuration spells it, which can include
// a credential, so the name is redacted wherever it appears.
func catalogFailures(err error) ([]catalogFailure, error) {
	if err == nil {
		return nil, nil
	}

	var loadErr *tui.SourceLoadError
	if !errors.As(err, &loadErr) {
		return nil, err
	}

	failures := make([]catalogFailure, 0, len(loadErr.Failures))

	for _, failure := range loadErr.Failures {
		source := redactAddress(failure.URL)

		failures = append(failures, catalogFailure{
			Source: source,
			Error: truncateRunes(
				strings.ReplaceAll(failure.Err.Error(), failure.URL, source),
				catalogFailureMaxRunes,
			),
		})
	}

	return failures, nil
}

// redactAddress returns addr with any credential it carries removed. An addr
// with neither a scheme nor a query is returned as written: it carries none,
// and rendering a local path as a URL would escape it.
func redactAddress(addr string) string {
	if !strings.Contains(addr, "://") && !strings.Contains(addr, "?") {
		return addr
	}

	return redact.NewURL(addr).String()
}

// catalogCollector is a [format.Renderer] that keeps the first limit
// components in their serialized form, and counts every one. The addresses a
// component names are redacted, since the configuration can spell its
// repository with a credential.
//
// [catalog.Stream] calls it from a single goroutine.
type catalogCollector struct {
	entries []*format.Entry
	limit   int
	found   int
	docs    catalogDocs
}

// Open implements [format.Renderer].
func (c *catalogCollector) Open(_ io.Writer) error {
	return nil
}

// Entry implements [format.Renderer].
func (c *catalogCollector) Entry(_ io.Writer, e *tui.ComponentEntry) error {
	c.found++

	if len(c.entries) >= c.limit {
		return nil
	}

	entry := format.NewEntry(e)
	entry.Source = redactAddress(entry.Source)
	entry.URL = redactAddress(entry.URL)
	entry.ComponentSource = redactAddress(entry.ComponentSource)

	c.entries = append(c.entries, entry)

	if c.docs == catalogOmitDocs {
		entry.Doc = ""

		return nil
	}

	entry.Doc = truncateRunes(entry.Doc, catalogDocMaxRunes)

	return nil
}

// Close implements [format.Renderer].
func (c *catalogCollector) Close(_ io.Writer, _ format.Summary) error {
	return nil
}
