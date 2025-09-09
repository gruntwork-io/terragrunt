package plaintext

import (
	"bytes"
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

func (render *Render) buildTemplate(templ string, customFuncs map[string]any) (*template.Template, error) {
	funcMap := template.FuncMap{}
	maps.Copy(funcMap, customFuncs)

	t := template.Must(template.New("template").Funcs(funcMap).Parse(templ))
	templates := map[string]string{
		"subcontrolTemplate":       subcontrolTemplate,
		"controlTemplate":          controlTemplate,
		"rangeSubcontrolsTemplate": rangeSubcontrolsTemplate,
		"rangeControlsTemplate":    rangeControlsTemplate,
	}

	for name, value := range templates {
		if _, err := t.New(name).Parse(value); err != nil {
			return nil, errors.Errorf("failed to parse template %s: %w", name, err)
		}
	}

	return t, nil
}

func (render *Render) formatOutput(t *template.Template, data any) (string, error) {
	out := new(bytes.Buffer)
	tabOut := tabwriter.NewWriter(out, tabMinWidth, tabWidth, tabPadding, ' ', 0)

	if err := t.Execute(tabOut, data); err != nil {
		return "", errors.Errorf("failed to execute template: %w", err)
	}

	if err := tabOut.Flush(); err != nil {
		return "", errors.Errorf("failed to flush output: %w", err)
	}

	return out.String(), nil
}

func (render *Render) executeTemplate(templ string, data any, customFuncs map[string]any) (string, error) {
	t, err := render.buildTemplate(templ, customFuncs)
	if err != nil {
		return "", err
	}

	return render.formatOutput(t, data)
}
