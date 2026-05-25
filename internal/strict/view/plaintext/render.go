package plaintext

import (
	"bytes"
	"io"
	"text/tabwriter"
	"text/template"

	"maps"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/view"
)

const (
	tabMinWidth = 1
	tabWidth    = 8
	tabPadding  = 2
)

var _ = view.Render(new(Render))

// subTemplates holds the parsed shared sub-templates. They're constants, so
// any parse failure is a programmer error and panics at init.
var subTemplates = func() *template.Template {
	t := template.New("subTemplates")
	template.Must(t.New("subcontrolTemplate").Parse(subcontrolTemplate))
	template.Must(t.New("controlTemplate").Parse(controlTemplate))
	template.Must(t.New("rangeSubcontrolsTemplate").Parse(rangeSubcontrolsTemplate))
	template.Must(t.New("rangeControlsTemplate").Parse(rangeControlsTemplate))

	return t
}()

// tabFlusher is the minimal subset of *tabwriter.Writer that formatOutput uses.
// It exists so tests can swap newTabFlusher for a stub whose Flush() returns
// a controlled error.
type tabFlusher interface {
	io.Writer
	Flush() error
}

var newTabFlusher = func(out io.Writer) tabFlusher {
	return tabwriter.NewWriter(out, tabMinWidth, tabWidth, tabPadding, ' ', 0)
}

type Render struct{}

func NewRender() *Render {
	return &Render{}
}

// List implements view.Render interface.
func (render *Render) List(controls strict.Controls) (string, error) {
	result, err := render.executeTemplate(listTemplate, map[string]any{
		"controls": controls,
	}, nil)
	if err != nil {
		return "", errors.Errorf("failed to render controls list: %w", err)
	}

	return result, nil
}

// DetailControl implements view.Render interface.
func (render *Render) DetailControl(control strict.Control) (string, error) {
	return render.executeTemplate(detailControlTemplate, map[string]any{"control": control}, nil)
}

func (render *Render) buildTemplate(templ string, customFuncs map[string]any) *template.Template {
	funcMap := template.FuncMap{}
	maps.Copy(funcMap, customFuncs)

	t := template.Must(subTemplates.Clone())
	template.Must(t.New("template").Funcs(funcMap).Parse(templ))

	return t
}

func (render *Render) formatOutput(t *template.Template, data any) (string, error) {
	out := new(bytes.Buffer)
	tabOut := newTabFlusher(out)

	if err := t.ExecuteTemplate(tabOut, "template", data); err != nil {
		return "", errors.Errorf("failed to execute template: %w", err)
	}

	if err := tabOut.Flush(); err != nil {
		return "", errors.Errorf("failed to flush output: %w", err)
	}

	return out.String(), nil
}

func (render *Render) executeTemplate(templ string, data any, customFuncs map[string]any) (string, error) {
	t := render.buildTemplate(templ, customFuncs)

	return render.formatOutput(t, data)
}
